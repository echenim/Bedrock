//! Deterministic serialization for execution boundary types.
//!
//! Uses a custom binary encoding for deterministic serialization.
//! All numeric values are little-endian (EXECUTION_SPEC.md §5.2).
//!
//! Encoding format:
//! - Fixed-size fields (Hash, u64, u32, bool) are written directly
//! - Variable-length fields (Vec<u8>, String) are length-prefixed (u32 LE)
//! - Repeated fields are count-prefixed (u32 LE) then concatenated
//! - Optional Hash: 1-byte flag (0=None, 1=Some) followed by 32 bytes if Some
//!
//! See EXECUTION_SPEC.md §3.1 (deterministic encoding requirement).

use alloc::string::String;
use alloc::vec::Vec;
use crate::error::ExecError;
use crate::execution::*;
use crate::types::Hash;

/// A cursor for reading bytes during decoding.
struct Reader<'a> {
    data: &'a [u8],
    pos: usize,
}

impl<'a> Reader<'a> {
    fn new(data: &'a [u8]) -> Self {
        Self { data, pos: 0 }
    }

    #[allow(dead_code)]
    fn remaining(&self) -> usize {
        self.data.len().saturating_sub(self.pos)
    }

    fn read_bytes(&mut self, n: usize) -> Result<&'a [u8], ExecError> {
        if self.pos + n > self.data.len() {
            return Err(ExecError::SerializationError(
                "unexpected end of data".into(),
            ));
        }
        let slice = &self.data[self.pos..self.pos + n];
        self.pos += n;
        Ok(slice)
    }

    fn read_u8(&mut self) -> Result<u8, ExecError> {
        let bytes = self.read_bytes(1)?;
        Ok(bytes[0])
    }

    fn read_u32(&mut self) -> Result<u32, ExecError> {
        let bytes = self.read_bytes(4)?;
        Ok(u32::from_le_bytes([bytes[0], bytes[1], bytes[2], bytes[3]]))
    }

    fn read_u64(&mut self) -> Result<u64, ExecError> {
        let bytes = self.read_bytes(8)?;
        let mut buf = [0u8; 8];
        buf.copy_from_slice(bytes);
        Ok(u64::from_le_bytes(buf))
    }

    fn read_bool(&mut self) -> Result<bool, ExecError> {
        let b = self.read_u8()?;
        match b {
            0 => Ok(false),
            1 => Ok(true),
            _ => Err(ExecError::SerializationError(
                "invalid bool value".into(),
            )),
        }
    }

    fn read_hash(&mut self) -> Result<Hash, ExecError> {
        let bytes = self.read_bytes(32)?;
        let mut hash = [0u8; 32];
        hash.copy_from_slice(bytes);
        Ok(hash)
    }

    fn read_optional_hash(&mut self) -> Result<Option<Hash>, ExecError> {
        let flag = self.read_u8()?;
        match flag {
            0 => Ok(None),
            1 => Ok(Some(self.read_hash()?)),
            _ => Err(ExecError::SerializationError(
                "invalid optional flag".into(),
            )),
        }
    }

    fn read_var_bytes(&mut self) -> Result<Vec<u8>, ExecError> {
        let len = self.read_u32()? as usize;
        Ok(self.read_bytes(len)?.to_vec())
    }

    fn read_string(&mut self) -> Result<String, ExecError> {
        let bytes = self.read_var_bytes()?;
        String::from_utf8(bytes)
            .map_err(|_| ExecError::SerializationError("invalid UTF-8".into()))
    }
}

// ── Encoding helpers ──

fn write_u8(buf: &mut Vec<u8>, v: u8) {
    buf.push(v);
}

fn write_u32(buf: &mut Vec<u8>, v: u32) {
    buf.extend_from_slice(&v.to_le_bytes());
}

fn write_u64(buf: &mut Vec<u8>, v: u64) {
    buf.extend_from_slice(&v.to_le_bytes());
}

fn write_bool(buf: &mut Vec<u8>, v: bool) {
    buf.push(if v { 1 } else { 0 });
}

fn write_hash(buf: &mut Vec<u8>, h: &Hash) {
    buf.extend_from_slice(h);
}

fn write_optional_hash(buf: &mut Vec<u8>, h: &Option<Hash>) {
    match h {
        None => buf.push(0),
        Some(hash) => {
            buf.push(1);
            buf.extend_from_slice(hash);
        }
    }
}

fn write_var_bytes(buf: &mut Vec<u8>, data: &[u8]) {
    write_u32(buf, data.len() as u32);
    buf.extend_from_slice(data);
}

fn write_string(buf: &mut Vec<u8>, s: &str) {
    write_var_bytes(buf, s.as_bytes());
}

// ── ExecutionRequest encoding ──

/// Encode an `ExecutionRequest` to deterministic bytes.
pub fn encode_execution_request(req: &ExecutionRequest) -> Vec<u8> {
    let mut buf = Vec::with_capacity(256);

    write_u32(&mut buf, req.api_version);
    write_var_bytes(&mut buf, &req.chain_id);
    write_u64(&mut buf, req.block_height);
    write_u64(&mut buf, req.block_time);
    write_hash(&mut buf, &req.block_hash);
    write_hash(&mut buf, &req.prev_state_root);

    // Transactions: count + each as var_bytes
    write_u32(&mut buf, req.transactions.len() as u32);
    for tx in &req.transactions {
        write_var_bytes(&mut buf, tx);
    }

    // Limits
    write_u64(&mut buf, req.limits.gas_limit);
    write_u32(&mut buf, req.limits.max_events);
    write_u32(&mut buf, req.limits.max_write_bytes);

    // Optional seed
    write_optional_hash(&mut buf, &req.execution_seed);

    buf
}

/// Decode an `ExecutionRequest` from bytes.
pub fn decode_execution_request(data: &[u8]) -> Result<ExecutionRequest, ExecError> {
    let mut r = Reader::new(data);

    let api_version = r.read_u32()?;
    let chain_id = r.read_var_bytes()?;
    let block_height = r.read_u64()?;
    let block_time = r.read_u64()?;
    let block_hash = r.read_hash()?;
    let prev_state_root = r.read_hash()?;

    let tx_count = r.read_u32()? as usize;
    let mut transactions = Vec::with_capacity(tx_count);
    for _ in 0..tx_count {
        transactions.push(r.read_var_bytes()?);
    }

    let gas_limit = r.read_u64()?;
    let max_events = r.read_u32()?;
    let max_write_bytes = r.read_u32()?;

    let execution_seed = r.read_optional_hash()?;

    Ok(ExecutionRequest {
        api_version,
        chain_id,
        block_height,
        block_time,
        block_hash,
        prev_state_root,
        transactions,
        limits: ExecutionLimits {
            gas_limit,
            max_events,
            max_write_bytes,
        },
        execution_seed,
    })
}

// ── ExecutionResponse encoding ──

/// Encode an `ExecutionResponse` to deterministic bytes.
pub fn encode_execution_response(resp: &ExecutionResponse) -> Vec<u8> {
    let mut buf = Vec::with_capacity(256);

    write_u32(&mut buf, resp.api_version);
    write_u8(&mut buf, resp.status as u8);
    write_hash(&mut buf, &resp.new_state_root);
    write_u64(&mut buf, resp.gas_used);

    // Receipts
    write_u32(&mut buf, resp.receipts.len() as u32);
    for receipt in &resp.receipts {
        encode_receipt(&mut buf, receipt);
    }

    // Events
    write_u32(&mut buf, resp.events.len() as u32);
    for event in &resp.events {
        encode_event(&mut buf, event);
    }

    // Logs
    write_u32(&mut buf, resp.logs.len() as u32);
    for log in &resp.logs {
        write_u32(&mut buf, log.level);
        write_string(&mut buf, &log.message);
    }

    buf
}

/// Decode an `ExecutionResponse` from bytes.
pub fn decode_execution_response(data: &[u8]) -> Result<ExecutionResponse, ExecError> {
    let mut r = Reader::new(data);

    let api_version = r.read_u32()?;
    let status_byte = r.read_u8()?;
    let status = ExecutionStatus::from_u8(status_byte).ok_or_else(|| {
        ExecError::SerializationError(alloc::format!("invalid status: {}", status_byte))
    })?;
    let new_state_root = r.read_hash()?;
    let gas_used = r.read_u64()?;

    let receipt_count = r.read_u32()? as usize;
    let mut receipts = Vec::with_capacity(receipt_count);
    for _ in 0..receipt_count {
        receipts.push(decode_receipt(&mut r)?);
    }

    let event_count = r.read_u32()? as usize;
    let mut events = Vec::with_capacity(event_count);
    for _ in 0..event_count {
        events.push(decode_event(&mut r)?);
    }

    let log_count = r.read_u32()? as usize;
    let mut logs = Vec::with_capacity(log_count);
    for _ in 0..log_count {
        let level = r.read_u32()?;
        let message = r.read_string()?;
        logs.push(LogLine { level, message });
    }

    Ok(ExecutionResponse {
        api_version,
        status,
        new_state_root,
        gas_used,
        receipts,
        events,
        logs,
    })
}

// ── Receipt encoding ──

fn encode_receipt(buf: &mut Vec<u8>, receipt: &Receipt) {
    write_u32(buf, receipt.tx_index);
    write_bool(buf, receipt.success);
    write_u64(buf, receipt.gas_used);
    write_u32(buf, receipt.result_code);
    write_var_bytes(buf, &receipt.return_data);
}

fn decode_receipt(r: &mut Reader<'_>) -> Result<Receipt, ExecError> {
    Ok(Receipt {
        tx_index: r.read_u32()?,
        success: r.read_bool()?,
        gas_used: r.read_u64()?,
        result_code: r.read_u32()?,
        return_data: r.read_var_bytes()?,
    })
}

// ── Event encoding ──

fn encode_event(buf: &mut Vec<u8>, event: &Event) {
    write_u32(buf, event.tx_index);
    write_string(buf, &event.event_type);

    write_u32(buf, event.attributes.len() as u32);
    for attr in &event.attributes {
        write_string(buf, &attr.key);
        write_var_bytes(buf, &attr.value);
    }
}

fn decode_event(r: &mut Reader<'_>) -> Result<Event, ExecError> {
    let tx_index = r.read_u32()?;
    let event_type = r.read_string()?;

    let attr_count = r.read_u32()? as usize;
    let mut attributes = Vec::with_capacity(attr_count);
    for _ in 0..attr_count {
        let key = r.read_string()?;
        let value = r.read_var_bytes()?;
        attributes.push(EventAttribute { key, value });
    }

    Ok(Event {
        tx_index,
        event_type,
        attributes,
    })
}

/// Encode a single `Event` to deterministic bytes.
/// Used by the WASM guest to serialize events for `emit_event` host call.
pub fn encode_single_event(event: &Event) -> Vec<u8> {
    let mut buf = Vec::with_capacity(64);
    encode_event(&mut buf, event);
    buf
}

/// Decode a single `Event` from bytes.
/// Used by the sandbox host to deserialize events from the guest.
pub fn decode_single_event(data: &[u8]) -> Result<Event, ExecError> {
    let mut r = Reader::new(data);
    decode_event(&mut r)
}

// ── ExecutionContext encoding ──

/// Encode an `ExecutionContext` to deterministic bytes.
/// Used by `get_context` host function.
pub fn encode_execution_context(ctx: &ExecutionContext) -> Vec<u8> {
    let mut buf = Vec::with_capacity(128);

    write_var_bytes(&mut buf, &ctx.chain_id);
    write_u64(&mut buf, ctx.block_height);
    write_u64(&mut buf, ctx.block_time);
    write_hash(&mut buf, &ctx.block_hash);
    write_u64(&mut buf, ctx.gas_limit);
    write_u32(&mut buf, ctx.max_events);
    write_u32(&mut buf, ctx.max_write_bytes);
    write_u32(&mut buf, ctx.api_version);
    write_optional_hash(&mut buf, &ctx.execution_seed);

    buf
}

/// Decode an `ExecutionContext` from bytes.
pub fn decode_execution_context(data: &[u8]) -> Result<ExecutionContext, ExecError> {
    let mut r = Reader::new(data);

    Ok(ExecutionContext {
        chain_id: r.read_var_bytes()?,
        block_height: r.read_u64()?,
        block_time: r.read_u64()?,
        block_hash: r.read_hash()?,
        gas_limit: r.read_u64()?,
        max_events: r.read_u32()?,
        max_write_bytes: r.read_u32()?,
        api_version: r.read_u32()?,
        execution_seed: r.read_optional_hash()?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{API_VERSION, ZERO_HASH};

    fn sample_request() -> ExecutionRequest {
        ExecutionRequest {
            api_version: API_VERSION,
            chain_id: b"bedrock-test".to_vec(),
            block_height: 42,
            block_time: 1_700_000_000,
            block_hash: [0xAA; 32],
            prev_state_root: [0xBB; 32],
            transactions: vec![b"tx1".to_vec(), b"tx2_longer".to_vec()],
            limits: ExecutionLimits {
                gas_limit: 5_000_000,
                max_events: 512,
                max_write_bytes: 2 * 1024 * 1024,
            },
            execution_seed: Some([0xCC; 32]),
        }
    }

    fn sample_response() -> ExecutionResponse {
        ExecutionResponse {
            api_version: API_VERSION,
            status: ExecutionStatus::Ok,
            new_state_root: [0xDD; 32],
            gas_used: 123_456,
            receipts: vec![
                Receipt {
                    tx_index: 0,
                    success: true,
                    gas_used: 50_000,
                    result_code: 0,
                    return_data: vec![1, 2, 3],
                },
                Receipt {
                    tx_index: 1,
                    success: false,
                    gas_used: 73_456,
                    result_code: 42,
                    return_data: vec![],
                },
            ],
            events: vec![Event {
                tx_index: 0,
                event_type: "transfer".into(),
                attributes: vec![
                    EventAttribute {
                        key: "from".into(),
                        value: b"alice".to_vec(),
                    },
                    EventAttribute {
                        key: "to".into(),
                        value: b"bob".to_vec(),
                    },
                    EventAttribute {
                        key: "amount".into(),
                        value: b"100".to_vec(),
                    },
                ],
            }],
            logs: vec![LogLine {
                level: 2,
                message: "block executed successfully".into(),
            }],
        }
    }

    #[test]
    fn test_request_roundtrip() {
        let req = sample_request();
        let encoded = encode_execution_request(&req);
        let decoded = decode_execution_request(&encoded).unwrap();
        assert_eq!(req, decoded);
    }

    #[test]
    fn test_response_roundtrip() {
        let resp = sample_response();
        let encoded = encode_execution_response(&resp);
        let decoded = decode_execution_response(&encoded).unwrap();
        assert_eq!(resp, decoded);
    }

    #[test]
    fn test_request_deterministic() {
        let req = sample_request();
        let enc1 = encode_execution_request(&req);
        let enc2 = encode_execution_request(&req);
        assert_eq!(enc1, enc2, "encoding must be deterministic");
    }

    #[test]
    fn test_response_deterministic() {
        let resp = sample_response();
        let enc1 = encode_execution_response(&resp);
        let enc2 = encode_execution_response(&resp);
        assert_eq!(enc1, enc2, "encoding must be deterministic");
    }

    #[test]
    fn test_encode_decode_encode_identical() {
        // encode → decode → encode must produce identical bytes
        let req = sample_request();
        let enc1 = encode_execution_request(&req);
        let decoded = decode_execution_request(&enc1).unwrap();
        let enc2 = encode_execution_request(&decoded);
        assert_eq!(enc1, enc2);

        let resp = sample_response();
        let enc1 = encode_execution_response(&resp);
        let decoded = decode_execution_response(&enc1).unwrap();
        let enc2 = encode_execution_response(&decoded);
        assert_eq!(enc1, enc2);
    }

    #[test]
    fn test_request_no_seed() {
        let mut req = sample_request();
        req.execution_seed = None;
        let encoded = encode_execution_request(&req);
        let decoded = decode_execution_request(&encoded).unwrap();
        assert_eq!(decoded.execution_seed, None);
    }

    #[test]
    fn test_request_no_transactions() {
        let mut req = sample_request();
        req.transactions = vec![];
        let encoded = encode_execution_request(&req);
        let decoded = decode_execution_request(&encoded).unwrap();
        assert!(decoded.transactions.is_empty());
    }

    #[test]
    fn test_response_failure_roundtrip() {
        let resp = ExecutionResponse::failure(API_VERSION, ExecutionStatus::OutOfGas, ZERO_HASH);
        let encoded = encode_execution_response(&resp);
        let decoded = decode_execution_response(&encoded).unwrap();
        assert_eq!(resp, decoded);
    }

    #[test]
    fn test_decode_truncated_data() {
        let err = decode_execution_request(&[0, 1, 2]);
        assert!(err.is_err());
    }

    #[test]
    fn test_context_roundtrip() {
        let req = sample_request();
        let ctx = ExecutionContext::from_request(&req);
        let encoded = encode_execution_context(&ctx);
        let decoded = decode_execution_context(&encoded).unwrap();
        assert_eq!(ctx, decoded);
    }

    #[test]
    fn test_context_deterministic() {
        let req = sample_request();
        let ctx = ExecutionContext::from_request(&req);
        let enc1 = encode_execution_context(&ctx);
        let enc2 = encode_execution_context(&ctx);
        assert_eq!(enc1, enc2);
    }
}
