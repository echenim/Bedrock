//! Host-side configuration types for the BedRock sandbox.
//!
//! `ExecutionConfig` bundles resource limits for a single block execution.
//! Default values match EXECUTION_SPEC.md §3.2 / §7.

use bedrock_primitives::{MAX_KEY_LEN, MAX_VALUE_LEN};

/// Configuration for a single block execution.
///
/// These limits are enforced by the host-side gas meter and HostApi
/// implementation. The guest cannot exceed them.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExecutionConfig {
    /// Maximum gas that can be consumed in this block.
    pub gas_limit: u64,
    /// Maximum number of events that can be emitted.
    pub max_events: u32,
    /// Maximum total bytes that can be written to state.
    pub max_write_bytes: u32,
    /// Maximum length of a state key in bytes.
    pub max_key_len: usize,
    /// Maximum length of a state value in bytes.
    pub max_value_len: usize,
    /// Maximum number of log lines allowed per execution.
    pub max_log_lines: u32,
    /// Maximum length of a single log line in bytes.
    pub max_log_line_len: usize,
}

impl Default for ExecutionConfig {
    fn default() -> Self {
        Self {
            gas_limit: 10_000_000,
            max_events: 1024,
            max_write_bytes: 4 * 1024 * 1024, // 4 MiB
            max_key_len: MAX_KEY_LEN,
            max_value_len: MAX_VALUE_LEN,
            max_log_lines: 256,
            max_log_line_len: 1024,
        }
    }
}

impl ExecutionConfig {
    /// Create a config from an `ExecutionLimits` and optional overrides.
    ///
    /// Uses the limits from the execution request for gas, events, and write bytes,
    /// and applies defaults for key/value/log limits.
    pub fn from_limits(limits: &bedrock_primitives::ExecutionLimits) -> Self {
        Self {
            gas_limit: limits.gas_limit,
            max_events: limits.max_events,
            max_write_bytes: limits.max_write_bytes,
            ..Self::default()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bedrock_primitives::ExecutionLimits;

    #[test]
    fn test_default_values() {
        let config = ExecutionConfig::default();
        assert_eq!(config.gas_limit, 10_000_000);
        assert_eq!(config.max_events, 1024);
        assert_eq!(config.max_write_bytes, 4 * 1024 * 1024);
        assert_eq!(config.max_key_len, MAX_KEY_LEN);
        assert_eq!(config.max_value_len, MAX_VALUE_LEN);
        assert_eq!(config.max_log_lines, 256);
        assert_eq!(config.max_log_line_len, 1024);
    }

    #[test]
    fn test_from_limits() {
        let limits = ExecutionLimits {
            gas_limit: 5_000_000,
            max_events: 512,
            max_write_bytes: 2 * 1024 * 1024,
        };
        let config = ExecutionConfig::from_limits(&limits);
        assert_eq!(config.gas_limit, 5_000_000);
        assert_eq!(config.max_events, 512);
        assert_eq!(config.max_write_bytes, 2 * 1024 * 1024);
        // Other fields should use defaults
        assert_eq!(config.max_key_len, MAX_KEY_LEN);
        assert_eq!(config.max_value_len, MAX_VALUE_LEN);
        assert_eq!(config.max_log_lines, 256);
        assert_eq!(config.max_log_line_len, 1024);
    }

    #[test]
    fn test_clone_eq() {
        let config = ExecutionConfig::default();
        let clone = config.clone();
        assert_eq!(config, clone);
    }
}
