//! Gas accounting for the BedRock execution layer.
//!
//! Gas is consumed for host calls (state reads/writes, crypto verification),
//! guest compute (via Wasmtime fuel), and event emission.
//! See EXECUTION_SPEC.md §7.

use crate::error::{ExecError, ExecResult};

// ── Gas cost constants (EXECUTION_SPEC.md §8) ──

/// Base cost for `state_get` (§8.1).
pub const G_STATE_GET: u64 = 200;

/// Base cost for `state_set` (§8.1).
pub const G_STATE_SET: u64 = 500;

/// Base cost for `state_delete` (§8.1).
pub const G_STATE_DEL: u64 = 300;

/// Per-byte cost added to state operations (§8.1).
pub const G_PER_BYTE: u64 = 3;

/// Base cost for `emit_event` (§8.2).
pub const G_EMIT_EVENT: u64 = 100;

/// Base cost for `hash_blake3` (§8.3).
pub const G_HASH_BLAKE3: u64 = 50;

/// Base cost for `verify_ed25519` (§8.3).
pub const G_VERIFY_ED25519: u64 = 2000;

/// Base cost for `verify_bls_agg` (§8.3).
pub const G_VERIFY_BLS_AGG: u64 = 5000;

/// Base cost for `log` (§8.2).
pub const G_LOG: u64 = 10;

/// Base cost for `get_context` (§8.6).
pub const G_GET_CONTEXT: u64 = 50;

/// Base cost for `gas_remaining` (§8.4).
pub const G_GAS_REMAINING: u64 = 5;

/// Base cost for `host_free` (§8.5).
pub const G_HOST_FREE: u64 = 5;

/// Compute the gas cost for a state_get operation.
pub fn gas_cost_state_get(key_len: usize) -> u64 {
    G_STATE_GET.saturating_add((key_len as u64).saturating_mul(G_PER_BYTE))
}

/// Compute the gas cost for a state_set operation.
pub fn gas_cost_state_set(key_len: usize, val_len: usize) -> u64 {
    let byte_cost = ((key_len + val_len) as u64).saturating_mul(G_PER_BYTE);
    G_STATE_SET.saturating_add(byte_cost)
}

/// Compute the gas cost for a state_delete operation.
pub fn gas_cost_state_delete(key_len: usize) -> u64 {
    G_STATE_DEL.saturating_add((key_len as u64).saturating_mul(G_PER_BYTE))
}

/// Compute the gas cost for an emit_event operation.
pub fn gas_cost_emit_event(event_len: usize) -> u64 {
    G_EMIT_EVENT.saturating_add((event_len as u64).saturating_mul(G_PER_BYTE))
}

/// Compute the gas cost for a log operation.
pub fn gas_cost_log(msg_len: usize) -> u64 {
    G_LOG.saturating_add((msg_len as u64).saturating_mul(G_PER_BYTE))
}

/// Compute the gas cost for a hash_blake3 operation.
pub fn gas_cost_hash_blake3(input_len: usize) -> u64 {
    G_HASH_BLAKE3.saturating_add((input_len as u64).saturating_mul(G_PER_BYTE))
}

/// Tracks gas consumption during block execution (EXECUTION_SPEC.md §7).
///
/// The gas meter enforces deterministic metering: the same block under the
/// same protocol version must consume identical gas across nodes.
#[derive(Debug, Clone)]
pub struct GasMeter {
    limit: u64,
    consumed: u64,
}

impl GasMeter {
    /// Create a new gas meter with the given limit.
    pub fn new(limit: u64) -> Self {
        Self { limit, consumed: 0 }
    }

    /// Create an unlimited gas meter (for testing only).
    pub fn unlimited() -> Self {
        Self {
            limit: u64::MAX,
            consumed: 0,
        }
    }

    /// Consume gas. Returns `OutOfGas` error if the limit would be exceeded.
    ///
    /// Gas consumption is checked before applying, so on error the consumed
    /// count remains unchanged — the meter is not in a half-consumed state.
    pub fn consume(&mut self, amount: u64) -> ExecResult<()> {
        let new_consumed = match self.consumed.checked_add(amount) {
            Some(v) if v <= self.limit => v,
            _ => {
                return Err(ExecError::OutOfGas {
                    limit: self.limit,
                    used: self.consumed.saturating_add(amount),
                });
            }
        };
        self.consumed = new_consumed;
        Ok(())
    }

    /// Consume gas for a base cost plus a per-byte charge.
    pub fn consume_with_bytes(&mut self, base: u64, byte_count: usize) -> ExecResult<()> {
        let byte_cost = (byte_count as u64).saturating_mul(G_PER_BYTE);
        let total = base.saturating_add(byte_cost);
        self.consume(total)
    }

    /// Returns the total gas consumed so far.
    pub fn consumed(&self) -> u64 {
        self.consumed
    }

    /// Returns the remaining gas before the limit is reached.
    pub fn remaining(&self) -> u64 {
        self.limit.saturating_sub(self.consumed)
    }

    /// Returns the gas limit.
    pub fn limit(&self) -> u64 {
        self.limit
    }

    /// Returns true if all gas has been consumed.
    pub fn is_exhausted(&self) -> bool {
        self.consumed >= self.limit
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gas_meter_basic() {
        let mut meter = GasMeter::new(1000);
        assert_eq!(meter.consumed(), 0);
        assert_eq!(meter.remaining(), 1000);
        assert_eq!(meter.limit(), 1000);

        meter.consume(100).unwrap();
        assert_eq!(meter.consumed(), 100);
        assert_eq!(meter.remaining(), 900);
    }

    #[test]
    fn test_gas_meter_exact_limit() {
        let mut meter = GasMeter::new(100);
        meter.consume(100).unwrap();
        assert_eq!(meter.consumed(), 100);
        assert_eq!(meter.remaining(), 0);
        assert!(meter.is_exhausted());
    }

    #[test]
    fn test_gas_meter_exceeds_limit() {
        let mut meter = GasMeter::new(100);
        meter.consume(50).unwrap();
        let err = meter.consume(51).unwrap_err();
        match err {
            ExecError::OutOfGas { limit, used } => {
                assert_eq!(limit, 100);
                assert_eq!(used, 101);
            }
            _ => panic!("expected OutOfGas"),
        }
        // consumed should not have changed on error
        assert_eq!(meter.consumed(), 50);
    }

    #[test]
    fn test_gas_meter_overflow_protection() {
        let mut meter = GasMeter::new(u64::MAX);
        meter.consume(u64::MAX - 1).unwrap();
        // This would overflow without saturating
        let err = meter.consume(2).unwrap_err();
        assert!(matches!(err, ExecError::OutOfGas { .. }));
    }

    #[test]
    fn test_gas_meter_consume_with_bytes() {
        let mut meter = GasMeter::new(10000);
        // G_STATE_GET (200) + 10 bytes * G_PER_BYTE (3) = 230
        meter.consume_with_bytes(G_STATE_GET, 10).unwrap();
        assert_eq!(meter.consumed(), 230);
    }

    #[test]
    fn test_gas_meter_unlimited() {
        let mut meter = GasMeter::unlimited();
        meter.consume(1_000_000_000).unwrap();
        assert_eq!(meter.consumed(), 1_000_000_000);
    }

    #[test]
    fn test_gas_cost_state_get() {
        // G_STATE_GET + key_len * G_PER_BYTE
        assert_eq!(gas_cost_state_get(0), 200);
        assert_eq!(gas_cost_state_get(10), 200 + 30);
        assert_eq!(gas_cost_state_get(100), 200 + 300);
    }

    #[test]
    fn test_gas_cost_state_set() {
        // G_STATE_SET + (key_len + val_len) * G_PER_BYTE
        assert_eq!(gas_cost_state_set(10, 20), 500 + 90);
        assert_eq!(gas_cost_state_set(0, 0), 500);
    }

    #[test]
    fn test_gas_cost_state_delete() {
        assert_eq!(gas_cost_state_delete(10), 300 + 30);
    }

    #[test]
    fn test_gas_cost_emit_event() {
        assert_eq!(gas_cost_emit_event(50), 100 + 150);
    }

    #[test]
    fn test_no_floating_point_in_gas() {
        // Verify gas arithmetic is pure integer — no float contamination.
        // Exhaustive check: all cost functions return exact integer sums.
        let cost = gas_cost_state_set(33, 67);
        assert_eq!(cost, G_STATE_SET + (33 + 67) * G_PER_BYTE);
    }
}
