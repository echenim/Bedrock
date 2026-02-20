//! Error types for the BedRock execution layer.
//!
//! Error codes are defined in EXECUTION_SPEC.md §9 (normative).

use alloc::string::String;
use core::fmt;

/// Host API error codes (EXECUTION_SPEC.md §9, normative).
///
/// All host functions return `i32` error codes. `0` = OK, non-zero = error.
/// These repr values MUST match the spec exactly.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
#[repr(i32)]
pub enum ErrorCode {
    Ok = 0,
    BadPointer = 1,
    InvalidEncoding = 2,
    KeyTooLarge = 3,
    ValueTooLarge = 4,
    WriteLimit = 5,
    EventLimit = 6,
    OutOfGas = 7,
    SigInvalid = 8,
    CryptoFailed = 9,
    Internal = 10,
}

impl ErrorCode {
    /// Convert from an i32 error code returned by a host function.
    pub fn from_i32(code: i32) -> Option<Self> {
        match code {
            0 => Some(Self::Ok),
            1 => Some(Self::BadPointer),
            2 => Some(Self::InvalidEncoding),
            3 => Some(Self::KeyTooLarge),
            4 => Some(Self::ValueTooLarge),
            5 => Some(Self::WriteLimit),
            6 => Some(Self::EventLimit),
            7 => Some(Self::OutOfGas),
            8 => Some(Self::SigInvalid),
            9 => Some(Self::CryptoFailed),
            10 => Some(Self::Internal),
            _ => None,
        }
    }

    /// Return the i32 representation of this error code.
    pub fn as_i32(self) -> i32 {
        self as i32
    }

    /// Returns true if this is the `Ok` variant.
    pub fn is_ok(self) -> bool {
        matches!(self, Self::Ok)
    }
}

impl fmt::Display for ErrorCode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Ok => write!(f, "OK"),
            Self::BadPointer => write!(f, "ERR_BAD_POINTER"),
            Self::InvalidEncoding => write!(f, "ERR_INVALID_ENCODING"),
            Self::KeyTooLarge => write!(f, "ERR_KEY_TOO_LARGE"),
            Self::ValueTooLarge => write!(f, "ERR_VALUE_TOO_LARGE"),
            Self::WriteLimit => write!(f, "ERR_WRITE_LIMIT"),
            Self::EventLimit => write!(f, "ERR_EVENT_LIMIT"),
            Self::OutOfGas => write!(f, "ERR_OUT_OF_GAS"),
            Self::SigInvalid => write!(f, "ERR_SIG_INVALID"),
            Self::CryptoFailed => write!(f, "ERR_CRYPTO_FAILED"),
            Self::Internal => write!(f, "ERR_INTERNAL"),
        }
    }
}

/// Execution engine error type.
///
/// This is the primary error type used throughout the execution layer.
/// It covers host errors, serialization failures, validation issues,
/// and resource exhaustion.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ExecError {
    /// Error returned by a host API call.
    HostError(ErrorCode),

    /// Serialization or deserialization failed.
    SerializationError(String),

    /// API version mismatch between host and guest.
    InvalidApiVersion { expected: u32, got: u32 },

    /// Block validation failed.
    InvalidBlock(String),

    /// Gas limit exceeded.
    OutOfGas { limit: u64, used: u64 },

    /// Merkle tree operation failed.
    MerkleError(String),
}

impl fmt::Display for ExecError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::HostError(code) => write!(f, "host error: {}", code),
            Self::SerializationError(msg) => write!(f, "serialization error: {}", msg),
            Self::InvalidApiVersion { expected, got } => {
                write!(f, "API version mismatch: expected {}, got {}", expected, got)
            }
            Self::InvalidBlock(msg) => write!(f, "invalid block: {}", msg),
            Self::OutOfGas { limit, used } => {
                write!(f, "out of gas: limit={}, used={}", limit, used)
            }
            Self::MerkleError(msg) => write!(f, "merkle error: {}", msg),
        }
    }
}

impl From<ErrorCode> for ExecError {
    fn from(code: ErrorCode) -> Self {
        Self::HostError(code)
    }
}

/// Convenience result type for the execution layer.
pub type ExecResult<T> = core::result::Result<T, ExecError>;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_error_code_repr_values() {
        // These MUST match EXECUTION_SPEC.md §9 exactly
        assert_eq!(ErrorCode::Ok as i32, 0);
        assert_eq!(ErrorCode::BadPointer as i32, 1);
        assert_eq!(ErrorCode::InvalidEncoding as i32, 2);
        assert_eq!(ErrorCode::KeyTooLarge as i32, 3);
        assert_eq!(ErrorCode::ValueTooLarge as i32, 4);
        assert_eq!(ErrorCode::WriteLimit as i32, 5);
        assert_eq!(ErrorCode::EventLimit as i32, 6);
        assert_eq!(ErrorCode::OutOfGas as i32, 7);
        assert_eq!(ErrorCode::SigInvalid as i32, 8);
        assert_eq!(ErrorCode::CryptoFailed as i32, 9);
        assert_eq!(ErrorCode::Internal as i32, 10);
    }

    #[test]
    fn test_error_code_from_i32_roundtrip() {
        for code in 0..=10 {
            let ec = ErrorCode::from_i32(code).unwrap();
            assert_eq!(ec.as_i32(), code);
        }
    }

    #[test]
    fn test_error_code_from_i32_invalid() {
        assert_eq!(ErrorCode::from_i32(-1), None);
        assert_eq!(ErrorCode::from_i32(11), None);
        assert_eq!(ErrorCode::from_i32(255), None);
    }

    #[test]
    fn test_error_code_is_ok() {
        assert!(ErrorCode::Ok.is_ok());
        assert!(!ErrorCode::OutOfGas.is_ok());
    }

    #[test]
    fn test_exec_error_from_error_code() {
        let err: ExecError = ErrorCode::OutOfGas.into();
        assert_eq!(err, ExecError::HostError(ErrorCode::OutOfGas));
    }

    #[test]
    fn test_exec_error_display() {
        let err = ExecError::OutOfGas {
            limit: 1000,
            used: 1500,
        };
        let s = alloc::format!("{}", err);
        assert!(s.contains("1000"));
        assert!(s.contains("1500"));
    }
}
