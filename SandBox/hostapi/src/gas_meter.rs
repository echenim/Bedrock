//! Host-side gas meter for the BedRock sandbox.
//!
//! The `HostGasMeter` is the authoritative gas counter. The guest may track
//! gas separately via `gas_remaining()`, but the host enforces limits.
//!
//! Gas charging uses the constants from `bedrock_primitives::gas`.
//! See EXECUTION_SPEC.md §7.

use bedrock_primitives::gas::G_PER_BYTE;
use crate::error::HostError;

/// Host-side gas meter — enforces gas limits for block execution.
///
/// This is the source of truth for gas accounting. Gas consumption is
/// checked before applying, so on error the consumed count remains unchanged.
#[derive(Debug, Clone)]
pub struct HostGasMeter {
    limit: u64,
    consumed: u64,
}

impl HostGasMeter {
    /// Create a new gas meter with the given limit.
    pub fn new(limit: u64) -> Self {
        Self { limit, consumed: 0 }
    }

    /// Charge gas. Returns `Err(OutOfGas)` if the limit would be exceeded.
    ///
    /// On error, the consumed count is NOT modified — the meter remains in
    /// its pre-charge state.
    pub fn charge(&mut self, amount: u64) -> Result<(), HostError> {
        let new_consumed = match self.consumed.checked_add(amount) {
            Some(v) if v <= self.limit => v,
            _ => return Err(HostError::out_of_gas()),
        };
        self.consumed = new_consumed;
        Ok(())
    }

    /// Charge gas proportional to data size.
    ///
    /// Formula: `base_cost + (byte_count * G_PER_BYTE)`
    pub fn charge_with_bytes(
        &mut self,
        base: u64,
        byte_count: usize,
    ) -> Result<(), HostError> {
        let byte_cost = (byte_count as u64).saturating_mul(G_PER_BYTE);
        let total = base.saturating_add(byte_cost);
        self.charge(total)
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
    use bedrock_primitives::gas::*;

    #[test]
    fn test_basic_charge() {
        let mut meter = HostGasMeter::new(1000);
        assert_eq!(meter.consumed(), 0);
        assert_eq!(meter.remaining(), 1000);
        assert_eq!(meter.limit(), 1000);

        meter.charge(100).unwrap();
        assert_eq!(meter.consumed(), 100);
        assert_eq!(meter.remaining(), 900);
    }

    #[test]
    fn test_exact_limit() {
        let mut meter = HostGasMeter::new(500);
        meter.charge(500).unwrap();
        assert_eq!(meter.consumed(), 500);
        assert_eq!(meter.remaining(), 0);
        assert!(meter.is_exhausted());
    }

    #[test]
    fn test_exceeds_limit() {
        let mut meter = HostGasMeter::new(100);
        meter.charge(60).unwrap();
        let err = meter.charge(41).unwrap_err();
        assert_eq!(err.to_error_code(), 7); // ERR_OUT_OF_GAS
        // consumed must not change on error
        assert_eq!(meter.consumed(), 60);
    }

    #[test]
    fn test_charge_with_bytes() {
        let mut meter = HostGasMeter::new(100_000);
        // G_STATE_GET (200) + 10 bytes * G_PER_BYTE (3) = 230
        meter.charge_with_bytes(G_STATE_GET, 10).unwrap();
        assert_eq!(meter.consumed(), 230);
    }

    #[test]
    fn test_charge_with_bytes_state_set() {
        let mut meter = HostGasMeter::new(100_000);
        // G_STATE_SET (500) + (10 + 20) bytes * G_PER_BYTE (3) = 590
        meter.charge_with_bytes(G_STATE_SET, 30).unwrap();
        assert_eq!(meter.consumed(), 590);
    }

    #[test]
    fn test_overflow_protection() {
        let mut meter = HostGasMeter::new(u64::MAX);
        meter.charge(u64::MAX - 1).unwrap();
        // This would overflow u64 without checked_add
        let err = meter.charge(2).unwrap_err();
        assert_eq!(err.to_error_code(), 7);
        // Consumed must not change
        assert_eq!(meter.consumed(), u64::MAX - 1);
    }

    #[test]
    fn test_zero_charge() {
        let mut meter = HostGasMeter::new(100);
        meter.charge(0).unwrap();
        assert_eq!(meter.consumed(), 0);
    }

    #[test]
    fn test_sequential_charges() {
        let mut meter = HostGasMeter::new(10_000);
        meter.charge_with_bytes(G_STATE_GET, 32).unwrap(); // 200 + 96 = 296
        meter.charge_with_bytes(G_STATE_SET, 64).unwrap(); // 500 + 192 = 692
        meter.charge(G_EMIT_EVENT).unwrap(); // 100
        assert_eq!(meter.consumed(), 296 + 692 + 100);
    }

    #[test]
    fn test_charge_with_bytes_overflow_bytes() {
        let mut meter = HostGasMeter::new(1000);
        // Very large byte count should fail with out of gas, not panic
        let err = meter.charge_with_bytes(100, usize::MAX).unwrap_err();
        assert_eq!(err.to_error_code(), 7);
        assert_eq!(meter.consumed(), 0);
    }
}
