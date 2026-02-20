//! Host-side error types for the BedRock sandbox.
//!
//! `HostError` is the primary error type used by the HostApi trait.
//! It wraps `ErrorCode` from `bedrock-primitives` for spec-defined errors
//! and provides an `Internal` variant for host-only errors not exposed to guests.
//!
//! See EXECUTION_SPEC.md §9 for the error code table.

use bedrock_primitives::ErrorCode;
use std::fmt;

/// Host-side error type returned by HostApi methods.
///
/// Guests see the `i32` error code via [`to_error_code`](HostError::to_error_code).
/// The `Internal` variant is mapped to `ErrorCode::Internal` (10) for the guest
/// but carries a descriptive message for host-side debugging.
#[derive(Debug, Clone)]
pub enum HostError {
    /// A spec-defined error code (EXECUTION_SPEC.md §9).
    Code(ErrorCode),
    /// An internal host error not directly mapped to a spec code.
    /// Returned to the guest as `ERR_INTERNAL` (10).
    Internal(String),
}

impl HostError {
    /// Convert to the `i32` error code returned to the WASM guest.
    pub fn to_error_code(&self) -> i32 {
        match self {
            Self::Code(code) => code.as_i32(),
            Self::Internal(_) => ErrorCode::Internal as i32,
        }
    }

    /// Create an out-of-gas error.
    pub fn out_of_gas() -> Self {
        Self::Code(ErrorCode::OutOfGas)
    }

    /// Create a bad-pointer error.
    pub fn bad_pointer() -> Self {
        Self::Code(ErrorCode::BadPointer)
    }

    /// Create a key-too-large error.
    pub fn key_too_large() -> Self {
        Self::Code(ErrorCode::KeyTooLarge)
    }

    /// Create a value-too-large error.
    pub fn value_too_large() -> Self {
        Self::Code(ErrorCode::ValueTooLarge)
    }

    /// Create a write-limit error.
    pub fn write_limit() -> Self {
        Self::Code(ErrorCode::WriteLimit)
    }

    /// Create an event-limit error.
    pub fn event_limit() -> Self {
        Self::Code(ErrorCode::EventLimit)
    }

    /// Create a signature-invalid error.
    pub fn sig_invalid() -> Self {
        Self::Code(ErrorCode::SigInvalid)
    }

    /// Create a crypto-failed error.
    pub fn crypto_failed() -> Self {
        Self::Code(ErrorCode::CryptoFailed)
    }

    /// Create an invalid-encoding error.
    pub fn invalid_encoding() -> Self {
        Self::Code(ErrorCode::InvalidEncoding)
    }
}

impl fmt::Display for HostError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Code(code) => write!(f, "host error: {}", code),
            Self::Internal(msg) => write!(f, "internal host error: {}", msg),
        }
    }
}

impl std::error::Error for HostError {}

impl From<ErrorCode> for HostError {
    fn from(code: ErrorCode) -> Self {
        Self::Code(code)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_error_code_conversion() {
        let err = HostError::Code(ErrorCode::OutOfGas);
        assert_eq!(err.to_error_code(), 7);

        let err = HostError::Code(ErrorCode::BadPointer);
        assert_eq!(err.to_error_code(), 1);

        let err = HostError::Code(ErrorCode::Ok);
        assert_eq!(err.to_error_code(), 0);
    }

    #[test]
    fn test_internal_maps_to_err_internal() {
        let err = HostError::Internal("something broke".into());
        assert_eq!(err.to_error_code(), 10); // ERR_INTERNAL
    }

    #[test]
    fn test_all_code_variants() {
        let cases: &[(ErrorCode, i32)] = &[
            (ErrorCode::Ok, 0),
            (ErrorCode::BadPointer, 1),
            (ErrorCode::InvalidEncoding, 2),
            (ErrorCode::KeyTooLarge, 3),
            (ErrorCode::ValueTooLarge, 4),
            (ErrorCode::WriteLimit, 5),
            (ErrorCode::EventLimit, 6),
            (ErrorCode::OutOfGas, 7),
            (ErrorCode::SigInvalid, 8),
            (ErrorCode::CryptoFailed, 9),
            (ErrorCode::Internal, 10),
        ];
        for &(code, expected) in cases {
            assert_eq!(HostError::Code(code).to_error_code(), expected);
        }
    }

    #[test]
    fn test_convenience_constructors() {
        assert_eq!(HostError::out_of_gas().to_error_code(), 7);
        assert_eq!(HostError::bad_pointer().to_error_code(), 1);
        assert_eq!(HostError::key_too_large().to_error_code(), 3);
        assert_eq!(HostError::value_too_large().to_error_code(), 4);
        assert_eq!(HostError::write_limit().to_error_code(), 5);
        assert_eq!(HostError::event_limit().to_error_code(), 6);
        assert_eq!(HostError::sig_invalid().to_error_code(), 8);
        assert_eq!(HostError::crypto_failed().to_error_code(), 9);
        assert_eq!(HostError::invalid_encoding().to_error_code(), 2);
    }

    #[test]
    fn test_display() {
        let err = HostError::Code(ErrorCode::OutOfGas);
        let s = format!("{}", err);
        assert!(s.contains("ERR_OUT_OF_GAS"));

        let err = HostError::Internal("disk full".into());
        let s = format!("{}", err);
        assert!(s.contains("disk full"));
    }

    #[test]
    fn test_from_error_code() {
        let err: HostError = ErrorCode::SigInvalid.into();
        assert_eq!(err.to_error_code(), 8);
    }
}
