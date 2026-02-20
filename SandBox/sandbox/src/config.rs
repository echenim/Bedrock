//! Sandbox configuration.

use bedrock_hostapi::ExecutionConfig;

/// Configuration for the WASM sandbox.
///
/// Controls memory limits, instruction fuel, and execution resource limits.
#[derive(Debug, Clone)]
pub struct SandboxConfig {
    /// Maximum linear memory pages (1 page = 64 KiB).
    /// Default: 256 pages = 16 MiB.
    pub max_memory_pages: u32,

    /// Wasmtime fuel limit (instruction metering).
    /// Prevents infinite loops in pure guest compute.
    pub fuel_limit: u64,

    /// Execution resource limits (gas, events, writes, etc.).
    pub execution: ExecutionConfig,

    /// Whether to collect debug logs from the guest.
    pub enable_guest_logs: bool,
}

impl Default for SandboxConfig {
    fn default() -> Self {
        Self {
            max_memory_pages: 256,       // 16 MiB
            fuel_limit: 100_000_000,
            execution: ExecutionConfig::default(),
            enable_guest_logs: false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = SandboxConfig::default();
        assert_eq!(config.max_memory_pages, 256);
        assert_eq!(config.fuel_limit, 100_000_000);
        assert!(!config.enable_guest_logs);
    }
}
