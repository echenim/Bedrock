//! WASM module validation — ABI compatibility checks.
//!
//! Validates that a compiled WASM module meets BedRock ABI requirements
//! before it can be used by the sandbox. Checks:
//!
//! 1. Required exports present with correct signatures
//! 2. All imports are from the `bedrock_host` module
//! 3. No WASI imports
//! 4. Memory export present
//!
//! See EXECUTION_SPEC.md §2.1, §12.

use wasmtime::{ExternType, Module, ValType};
use crate::error::SandboxError;

/// Check if a ValType is i32.
fn is_i32(vt: &ValType) -> bool {
    matches!(vt, ValType::I32)
}

/// Expected export: (name, param_count_of_i32, result_count_of_i32).
/// All params and results are i32 in the BedRock ABI.
const REQUIRED_EXPORTS: &[(&str, usize, usize)] = &[
    ("bedrock_init", 2, 1),
    ("bedrock_execute_block", 4, 1),
    ("bedrock_free", 2, 0),
];

/// Allowed import module name.
const ALLOWED_IMPORT_MODULE: &str = "bedrock_host";

/// Validate that a WASM module meets BedRock ABI requirements.
pub fn validate_module(module: &Module) -> Result<(), SandboxError> {
    validate_exports(module)?;
    validate_imports(module)?;
    Ok(())
}

/// Check that all required exports are present with correct signatures.
fn validate_exports(module: &Module) -> Result<(), SandboxError> {
    // Check for memory export
    let has_memory = module
        .exports()
        .any(|e| e.name() == "memory" && matches!(e.ty(), ExternType::Memory(_)));
    if !has_memory {
        return Err(SandboxError::ValidationError(
            "module must export 'memory'".into(),
        ));
    }

    // Check required function exports
    for &(name, expected_param_count, expected_result_count) in REQUIRED_EXPORTS {
        let export = module
            .exports()
            .find(|e| e.name() == name)
            .ok_or_else(|| {
                SandboxError::ValidationError(format!("missing required export: {}", name))
            })?;

        let func_ty = match export.ty() {
            ExternType::Func(ft) => ft,
            _ => {
                return Err(SandboxError::ValidationError(format!(
                    "export '{}' must be a function",
                    name
                )));
            }
        };

        let params: Vec<ValType> = func_ty.params().collect();
        let results: Vec<ValType> = func_ty.results().collect();

        // All BedRock ABI params and results are i32
        if params.len() != expected_param_count || !params.iter().all(is_i32) {
            return Err(SandboxError::ValidationError(format!(
                "export '{}' has wrong param signature: expected {} i32 params, got {} params",
                name, expected_param_count, params.len()
            )));
        }

        if results.len() != expected_result_count || !results.iter().all(is_i32) {
            return Err(SandboxError::ValidationError(format!(
                "export '{}' has wrong result signature: expected {} i32 results, got {} results",
                name, expected_result_count, results.len()
            )));
        }
    }

    Ok(())
}

/// Check that all imports are from `bedrock_host` and none are WASI.
fn validate_imports(module: &Module) -> Result<(), SandboxError> {
    for import in module.imports() {
        let module_name = import.module();

        // Reject WASI imports
        if module_name.starts_with("wasi") {
            return Err(SandboxError::ValidationError(format!(
                "WASI import not allowed: {}::{}",
                module_name,
                import.name()
            )));
        }

        // All imports must be from bedrock_host
        if module_name != ALLOWED_IMPORT_MODULE {
            return Err(SandboxError::ValidationError(format!(
                "import from unknown module '{}' (only '{}' allowed): {}",
                module_name,
                ALLOWED_IMPORT_MODULE,
                import.name()
            )));
        }

        // Imports must be functions
        if !matches!(import.ty(), ExternType::Func(_)) {
            return Err(SandboxError::ValidationError(format!(
                "non-function import not allowed: {}::{}",
                module_name,
                import.name()
            )));
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use wasmtime::Engine;

    fn test_engine() -> Engine {
        Engine::default()
    }

    #[test]
    fn test_validate_minimal_valid_module() {
        // A minimal module with required exports and no imports
        let wat = r#"
            (module
                (memory (export "memory") 1)
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_free") (param i32 i32))
            )
        "#;
        let engine = test_engine();
        let module = Module::new(&engine, wat).unwrap();
        validate_module(&module).unwrap();
    }

    #[test]
    fn test_reject_missing_export() {
        let wat = r#"
            (module
                (memory (export "memory") 1)
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
                ;; Missing bedrock_execute_block and bedrock_free
            )
        "#;
        let engine = test_engine();
        let module = Module::new(&engine, wat).unwrap();
        let err = validate_module(&module).unwrap_err();
        assert!(matches!(err, SandboxError::ValidationError(_)));
    }

    #[test]
    fn test_reject_wrong_signature() {
        let wat = r#"
            (module
                (memory (export "memory") 1)
                ;; Wrong signature: bedrock_init should take 2 params, not 1
                (func (export "bedrock_init") (param i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_free") (param i32 i32))
            )
        "#;
        let engine = test_engine();
        let module = Module::new(&engine, wat).unwrap();
        let err = validate_module(&module).unwrap_err();
        assert!(matches!(err, SandboxError::ValidationError(_)));
    }

    #[test]
    fn test_reject_missing_memory() {
        let wat = r#"
            (module
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_free") (param i32 i32))
            )
        "#;
        let engine = test_engine();
        let module = Module::new(&engine, wat).unwrap();
        let err = validate_module(&module).unwrap_err();
        assert!(matches!(err, SandboxError::ValidationError(_)));
    }

    #[test]
    fn test_reject_wasi_import() {
        let wat = r#"
            (module
                (import "wasi_snapshot_preview1" "fd_write"
                    (func (param i32 i32 i32 i32) (result i32)))
                (memory (export "memory") 1)
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_free") (param i32 i32))
            )
        "#;
        let engine = test_engine();
        let module = Module::new(&engine, wat).unwrap();
        let err = validate_module(&module).unwrap_err();
        assert!(matches!(err, SandboxError::ValidationError(_)));
    }

    #[test]
    fn test_accept_bedrock_host_import() {
        let wat = r#"
            (module
                (import "bedrock_host" "state_get"
                    (func (param i32 i32 i32 i32) (result i32)))
                (memory (export "memory") 1)
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_free") (param i32 i32))
            )
        "#;
        let engine = test_engine();
        let module = Module::new(&engine, wat).unwrap();
        validate_module(&module).unwrap();
    }

    #[test]
    fn test_reject_unknown_module_import() {
        let wat = r#"
            (module
                (import "env" "some_func" (func (result i32)))
                (memory (export "memory") 1)
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_free") (param i32 i32))
            )
        "#;
        let engine = test_engine();
        let module = Module::new(&engine, wat).unwrap();
        let err = validate_module(&module).unwrap_err();
        assert!(matches!(err, SandboxError::ValidationError(_)));
    }
}
