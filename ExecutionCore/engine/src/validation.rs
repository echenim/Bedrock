//! Request and block validation logic.
//!
//! These functions validate an `ExecutionRequest` before the engine
//! processes any transactions. Validation failures produce an
//! `InvalidBlock` status response without consuming gas.
//!
//! See EXECUTION_SPEC.md §2.2 (validation phase) and §10 (versioning).

use bedrock_primitives::{
    ExecError, ExecResult, ExecutionRequest,
    types::API_VERSION,
};

/// Validate the API version in an `ExecutionRequest`.
///
/// The guest must reject requests with unsupported versions (§10).
/// Returns `InvalidApiVersion` error on mismatch.
pub fn validate_api_version(request: &ExecutionRequest) -> ExecResult<()> {
    if request.api_version != API_VERSION {
        return Err(ExecError::InvalidApiVersion {
            expected: API_VERSION,
            got: request.api_version,
        });
    }
    Ok(())
}

/// Validate block-level fields in an `ExecutionRequest` (§2.2).
///
/// Checks:
/// - `block_height > 0` (genesis is height 0, execution starts at 1)
/// - `chain_id` is non-empty
/// - `gas_limit > 0`
pub fn validate_block_fields(request: &ExecutionRequest) -> ExecResult<()> {
    if request.block_height == 0 {
        return Err(ExecError::InvalidBlock(
            "block_height must be > 0".into(),
        ));
    }

    if request.chain_id.is_empty() {
        return Err(ExecError::InvalidBlock(
            "chain_id must be non-empty".into(),
        ));
    }

    if request.limits.gas_limit == 0 {
        return Err(ExecError::InvalidBlock(
            "gas_limit must be > 0".into(),
        ));
    }

    Ok(())
}

/// Validate an entire `ExecutionRequest` before processing.
///
/// Runs all validation checks in order:
/// 1. API version
/// 2. Block field consistency
pub fn validate_request(request: &ExecutionRequest) -> ExecResult<()> {
    validate_api_version(request)?;
    validate_block_fields(request)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use bedrock_primitives::{ExecutionLimits, types::ZERO_HASH};

    fn valid_request() -> ExecutionRequest {
        ExecutionRequest {
            api_version: API_VERSION,
            chain_id: b"bedrock-test".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: ZERO_HASH,
            prev_state_root: ZERO_HASH,
            transactions: Vec::new(),
            limits: ExecutionLimits::default(),
            execution_seed: None,
        }
    }

    #[test]
    fn test_valid_request_passes() {
        assert!(validate_request(&valid_request()).is_ok());
    }

    #[test]
    fn test_invalid_api_version() {
        let mut req = valid_request();
        req.api_version = 999;
        let err = validate_request(&req).unwrap_err();
        assert!(matches!(err, ExecError::InvalidApiVersion { expected: 1, got: 999 }));
    }

    #[test]
    fn test_zero_block_height() {
        let mut req = valid_request();
        req.block_height = 0;
        let err = validate_request(&req).unwrap_err();
        assert!(matches!(err, ExecError::InvalidBlock(_)));
    }

    #[test]
    fn test_empty_chain_id() {
        let mut req = valid_request();
        req.chain_id = Vec::new();
        let err = validate_request(&req).unwrap_err();
        assert!(matches!(err, ExecError::InvalidBlock(_)));
    }

    #[test]
    fn test_zero_gas_limit() {
        let mut req = valid_request();
        req.limits.gas_limit = 0;
        let err = validate_request(&req).unwrap_err();
        assert!(matches!(err, ExecError::InvalidBlock(_)));
    }

    #[test]
    fn test_api_version_standalone() {
        let req = valid_request();
        assert!(validate_api_version(&req).is_ok());

        let mut bad = valid_request();
        bad.api_version = 0;
        assert!(validate_api_version(&bad).is_err());
    }

    #[test]
    fn test_block_fields_standalone() {
        let req = valid_request();
        assert!(validate_block_fields(&req).is_ok());
    }

    #[test]
    fn test_high_block_height_valid() {
        let mut req = valid_request();
        req.block_height = u64::MAX;
        assert!(validate_request(&req).is_ok());
    }

    #[test]
    fn test_with_transactions_valid() {
        let mut req = valid_request();
        req.transactions = vec![b"tx1".to_vec(), b"tx2".to_vec()];
        assert!(validate_request(&req).is_ok());
    }
}
