//! Transaction decoding, validation, and processing.
//!
//! Transactions are received as opaque bytes in the `ExecutionRequest`.
//! This module decodes them into typed structures, validates signatures
//! and nonces, and processes state transitions.
//!
//! ## Wire Format (little-endian)
//!
//! ```text
//! [sender: 32 bytes]
//! [nonce: 8 bytes LE]
//! [payload_type: 1 byte]
//!   0x01 = Transfer { to: 32 bytes, amount: 8 bytes LE }
//! [public_key: 32 bytes]
//! [signature: 64 bytes]
//! ```
//!
//! The signature covers everything before the public_key field:
//! `sender || nonce || payload_type || payload_data`.

use bedrock_primitives::{
    Address, ExecError, ExecResult, Receipt, Event, EventAttribute,
    gas::{gas_cost_state_get, gas_cost_state_set, G_VERIFY_ED25519},
    types::{u64_from_le_bytes, u64_to_le_bytes},
};
use crate::host::HostInterface;
use alloc::string::String;
use alloc::vec::Vec;

/// Minimum valid transaction size:
/// sender(32) + nonce(8) + payload_type(1) + transfer_payload(40) + pubkey(32) + sig(64) = 177
const MIN_TRANSFER_TX_SIZE: usize = 32 + 8 + 1 + 32 + 8 + 32 + 64;

/// Payload type tag for transfers.
const PAYLOAD_TRANSFER: u8 = 0x01;

/// State key prefix for account balances.
const BALANCE_PREFIX: &[u8] = b"acct/";
/// State key suffix for account balances.
const BALANCE_SUFFIX: &[u8] = b"/balance";
/// State key suffix for account nonces.
const NONCE_SUFFIX: &[u8] = b"/nonce";

/// A decoded transaction ready for processing.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DecodedTransaction {
    /// Sender address (32 bytes).
    pub sender: Address,
    /// Sequence number for replay protection.
    pub nonce: u64,
    /// Transaction payload.
    pub payload: TransactionPayload,
    /// Ed25519 public key of the sender.
    pub public_key: [u8; 32],
    /// Ed25519 signature over the signed portion.
    pub signature: [u8; 64],
    /// The raw bytes that were signed (for verification).
    pub signed_data: Vec<u8>,
}

/// Transaction payload variants.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TransactionPayload {
    /// Transfer tokens from sender to recipient.
    Transfer {
        /// Recipient address.
        to: Address,
        /// Amount to transfer.
        amount: u64,
    },
}

/// Decode a raw transaction from bytes.
///
/// Returns `ExecError::SerializationError` if the bytes are malformed.
pub fn decode_transaction(raw: &[u8]) -> ExecResult<DecodedTransaction> {
    if raw.len() < MIN_TRANSFER_TX_SIZE {
        return Err(ExecError::SerializationError(
            "transaction too short".into(),
        ));
    }

    let mut pos = 0;

    // sender: 32 bytes
    let mut sender = [0u8; 32];
    sender.copy_from_slice(&raw[pos..pos + 32]);
    pos += 32;

    // nonce: 8 bytes LE
    let nonce = u64_from_le_bytes(&raw[pos..pos + 8])
        .ok_or_else(|| ExecError::SerializationError("invalid nonce".into()))?;
    pos += 8;

    // payload_type: 1 byte
    let payload_type = raw[pos];
    pos += 1;

    let payload = match payload_type {
        PAYLOAD_TRANSFER => {
            if raw.len() < pos + 32 + 8 + 32 + 64 {
                return Err(ExecError::SerializationError(
                    "transfer payload too short".into(),
                ));
            }
            let mut to = [0u8; 32];
            to.copy_from_slice(&raw[pos..pos + 32]);
            pos += 32;

            let amount = u64_from_le_bytes(&raw[pos..pos + 8])
                .ok_or_else(|| ExecError::SerializationError("invalid amount".into()))?;
            pos += 8;

            TransactionPayload::Transfer { to, amount }
        }
        _ => {
            return Err(ExecError::SerializationError(
                alloc::format!("unknown payload type: 0x{:02x}", payload_type),
            ));
        }
    };

    // signed_data = everything up to this point
    let signed_data = raw[..pos].to_vec();

    // public_key: 32 bytes
    if raw.len() < pos + 32 + 64 {
        return Err(ExecError::SerializationError(
            "missing public key or signature".into(),
        ));
    }
    let mut public_key = [0u8; 32];
    public_key.copy_from_slice(&raw[pos..pos + 32]);
    pos += 32;

    // signature: 64 bytes
    let mut signature = [0u8; 64];
    signature.copy_from_slice(&raw[pos..pos + 64]);

    Ok(DecodedTransaction {
        sender,
        nonce,
        payload,
        public_key,
        signature,
        signed_data,
    })
}

/// Build the state key for an account's balance.
fn balance_key(addr: &Address) -> Vec<u8> {
    let mut key = Vec::with_capacity(BALANCE_PREFIX.len() + 32 + BALANCE_SUFFIX.len());
    key.extend_from_slice(BALANCE_PREFIX);
    key.extend_from_slice(addr);
    key.extend_from_slice(BALANCE_SUFFIX);
    key
}

/// Build the state key for an account's nonce.
fn nonce_key(addr: &Address) -> Vec<u8> {
    let mut key = Vec::with_capacity(BALANCE_PREFIX.len() + 32 + NONCE_SUFFIX.len());
    key.extend_from_slice(BALANCE_PREFIX);
    key.extend_from_slice(addr);
    key.extend_from_slice(NONCE_SUFFIX);
    key
}

/// Read a u64 balance from state. Returns 0 if the key doesn't exist.
fn read_balance(host: &dyn HostInterface, addr: &Address) -> ExecResult<u64> {
    let key = balance_key(addr);
    match host.state_get(&key)? {
        Some(bytes) => u64_from_le_bytes(&bytes)
            .ok_or_else(|| ExecError::SerializationError("corrupt balance".into())),
        None => Ok(0),
    }
}

/// Read a u64 nonce from state. Returns 0 if the key doesn't exist.
fn read_nonce(host: &dyn HostInterface, addr: &Address) -> ExecResult<u64> {
    let key = nonce_key(addr);
    match host.state_get(&key)? {
        Some(bytes) => u64_from_le_bytes(&bytes)
            .ok_or_else(|| ExecError::SerializationError("corrupt nonce".into())),
        None => Ok(0),
    }
}

/// Write a u64 balance to state.
fn write_balance(host: &mut dyn HostInterface, addr: &Address, balance: u64) -> ExecResult<()> {
    let key = balance_key(addr);
    let value = u64_to_le_bytes(balance);
    host.state_set(&key, &value)
}

/// Write a u64 nonce to state.
fn write_nonce(host: &mut dyn HostInterface, addr: &Address, nonce: u64) -> ExecResult<()> {
    let key = nonce_key(addr);
    let value = u64_to_le_bytes(nonce);
    host.state_set(&key, &value)
}

/// Process a single decoded transaction against the host state.
///
/// Gas is metered through `host.gas_meter_mut()`. Steps:
/// 1. Charge gas for signature verification
/// 2. Verify Ed25519 signature
/// 3. Validate nonce (must match account's current nonce)
/// 4. Execute payload (transfer: check balance, update sender/receiver)
/// 5. Increment sender nonce
/// 6. Produce a `Receipt`
///
/// Individual transaction failures produce a failed receipt but do NOT
/// abort block execution (§2.3).
pub fn process_transaction(
    tx_index: u32,
    tx: &DecodedTransaction,
    host: &mut dyn HostInterface,
) -> Receipt {
    let gas_before = host.gas_meter().consumed();

    // Attempt to process; on any error, produce a failed receipt
    match process_transaction_inner(tx_index, tx, host) {
        Ok(events) => {
            // Emit events for successful transaction
            for event in events {
                // Event emission failure doesn't abort the tx
                let _ = host.emit_event(event);
            }
            Receipt {
                tx_index,
                success: true,
                gas_used: host.gas_meter().consumed() - gas_before,
                result_code: 0,
                return_data: Vec::new(),
            }
        }
        Err(err) => {
            let result_code = match &err {
                ExecError::HostError(code) => code.as_i32() as u32,
                ExecError::OutOfGas { .. } => 7,
                ExecError::InvalidBlock(_) => 1,
                ExecError::SerializationError(_) => 2,
                _ => 10,
            };
            Receipt {
                tx_index,
                success: false,
                gas_used: host.gas_meter().consumed() - gas_before,
                result_code,
                return_data: Vec::new(),
            }
        }
    }
}

/// Inner transaction processing that can return errors.
fn process_transaction_inner(
    tx_index: u32,
    tx: &DecodedTransaction,
    host: &mut dyn HostInterface,
) -> ExecResult<Vec<Event>> {
    // 1. Charge gas for signature verification
    host.gas_meter_mut().consume(G_VERIFY_ED25519)?;

    // 2. Verify Ed25519 signature
    let sig_valid = host.verify_ed25519(&tx.signed_data, &tx.signature, &tx.public_key)?;
    if !sig_valid {
        return Err(ExecError::HostError(bedrock_primitives::ErrorCode::SigInvalid));
    }

    // 3. Read and validate nonce
    let nonce_key_bytes = nonce_key(&tx.sender);
    let nonce_read_gas = gas_cost_state_get(nonce_key_bytes.len());
    host.gas_meter_mut().consume(nonce_read_gas)?;
    let current_nonce = read_nonce(host, &tx.sender)?;
    if tx.nonce != current_nonce {
        return Err(ExecError::InvalidBlock(
            alloc::format!(
                "nonce mismatch: expected {}, got {}",
                current_nonce, tx.nonce
            ),
        ));
    }

    // 4. Execute payload
    let events = match &tx.payload {
        TransactionPayload::Transfer { to, amount } => {
            execute_transfer(tx_index, &tx.sender, to, *amount, host)?
        }
    };

    // 5. Increment sender nonce
    let nonce_key_bytes = nonce_key(&tx.sender);
    let nonce_write_gas = gas_cost_state_set(nonce_key_bytes.len(), 8);
    host.gas_meter_mut().consume(nonce_write_gas)?;
    write_nonce(host, &tx.sender, current_nonce + 1)?;

    Ok(events)
}

/// Execute a transfer: debit sender, credit recipient.
fn execute_transfer(
    tx_index: u32,
    sender: &Address,
    to: &Address,
    amount: u64,
    host: &mut dyn HostInterface,
) -> ExecResult<Vec<Event>> {
    // Charge gas for reading sender balance
    let sender_bal_key = balance_key(sender);
    host.gas_meter_mut().consume(gas_cost_state_get(sender_bal_key.len()))?;
    let sender_balance = read_balance(host, sender)?;

    if sender_balance < amount {
        return Err(ExecError::InvalidBlock(
            alloc::format!(
                "insufficient balance: have {}, need {}",
                sender_balance, amount
            ),
        ));
    }

    // Charge gas for reading recipient balance
    let to_bal_key = balance_key(to);
    host.gas_meter_mut().consume(gas_cost_state_get(to_bal_key.len()))?;
    let to_balance = read_balance(host, to)?;

    // Check for overflow on recipient
    let new_to_balance = to_balance.checked_add(amount)
        .ok_or_else(|| ExecError::InvalidBlock("recipient balance overflow".into()))?;

    // Write updated balances
    host.gas_meter_mut().consume(gas_cost_state_set(sender_bal_key.len(), 8))?;
    write_balance(host, sender, sender_balance - amount)?;

    host.gas_meter_mut().consume(gas_cost_state_set(to_bal_key.len(), 8))?;
    write_balance(host, to, new_to_balance)?;

    // Produce transfer event
    let event = Event {
        tx_index,
        event_type: String::from("transfer"),
        attributes: alloc::vec![
            EventAttribute {
                key: String::from("sender"),
                value: sender.to_vec(),
            },
            EventAttribute {
                key: String::from("recipient"),
                value: to.to_vec(),
            },
            EventAttribute {
                key: String::from("amount"),
                value: u64_to_le_bytes(amount).to_vec(),
            },
        ],
    };

    Ok(alloc::vec![event])
}

/// Encode a transfer transaction into raw bytes for testing.
#[cfg(feature = "std")]
pub fn encode_transfer_tx(
    sender: &Address,
    nonce: u64,
    to: &Address,
    amount: u64,
    signing_key: &ed25519_dalek::SigningKey,
) -> Vec<u8> {
    let mut signed_data = Vec::new();
    signed_data.extend_from_slice(sender);
    signed_data.extend_from_slice(&u64_to_le_bytes(nonce));
    signed_data.push(PAYLOAD_TRANSFER);
    signed_data.extend_from_slice(to);
    signed_data.extend_from_slice(&u64_to_le_bytes(amount));

    let signature = bedrock_primitives::crypto::sign_ed25519(&signed_data, signing_key);
    let public_key = signing_key.verifying_key();

    let mut raw = signed_data;
    raw.extend_from_slice(public_key.as_bytes());
    raw.extend_from_slice(&signature);
    raw
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::host::MockHost;
    use alloc::collections::BTreeMap;
    use bedrock_primitives::{
        ExecutionContext,
        crypto::generate_keypair,
        types::API_VERSION,
    };

    fn test_context() -> ExecutionContext {
        ExecutionContext {
            chain_id: b"test".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: [0u8; 32],
            gas_limit: 10_000_000,
            max_events: 1024,
            max_write_bytes: 4 * 1024 * 1024,
            api_version: API_VERSION,
            execution_seed: None,
        }
    }

    fn setup_host_with_balance(addr: &Address, balance: u64) -> MockHost {
        let mut committed = BTreeMap::new();
        let key = balance_key(addr);
        committed.insert(key, u64_to_le_bytes(balance).to_vec());
        MockHost::new(committed, test_context())
    }

    #[test]
    fn test_decode_transfer_roundtrip() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];
        let raw = encode_transfer_tx(&sender, 0, &to, 1000, &sk);

        let decoded = decode_transaction(&raw).unwrap();
        assert_eq!(decoded.sender, sender);
        assert_eq!(decoded.nonce, 0);
        assert_eq!(decoded.public_key, *vk.as_bytes());
        match decoded.payload {
            TransactionPayload::Transfer { to: decoded_to, amount } => {
                assert_eq!(decoded_to, to);
                assert_eq!(amount, 1000);
            }
        }
    }

    #[test]
    fn test_decode_too_short() {
        let raw = vec![0u8; 10];
        assert!(decode_transaction(&raw).is_err());
    }

    #[test]
    fn test_decode_unknown_payload_type() {
        let mut raw = vec![0u8; MIN_TRANSFER_TX_SIZE];
        // Set payload type to unknown (at offset 32+8=40)
        raw[40] = 0xFF;
        assert!(decode_transaction(&raw).is_err());
    }

    #[test]
    fn test_process_valid_transfer() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_host_with_balance(&sender, 5000);
        let raw = encode_transfer_tx(&sender, 0, &to, 1000, &sk);
        let decoded = decode_transaction(&raw).unwrap();

        let receipt = process_transaction(0, &decoded, &mut host);

        assert!(receipt.success);
        assert_eq!(receipt.tx_index, 0);
        assert!(receipt.gas_used > 0);

        // Verify balances
        assert_eq!(read_balance(&host, &sender).unwrap(), 4000);
        assert_eq!(read_balance(&host, &to).unwrap(), 1000);

        // Verify nonce incremented
        assert_eq!(read_nonce(&host, &sender).unwrap(), 1);
    }

    #[test]
    fn test_process_invalid_signature() {
        let (vk, _sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_host_with_balance(&sender, 5000);

        // Create a transaction with a bad signature
        let mut signed_data = Vec::new();
        signed_data.extend_from_slice(&sender);
        signed_data.extend_from_slice(&u64_to_le_bytes(0));
        signed_data.push(PAYLOAD_TRANSFER);
        signed_data.extend_from_slice(&to);
        signed_data.extend_from_slice(&u64_to_le_bytes(1000));

        let mut raw = signed_data;
        raw.extend_from_slice(vk.as_bytes());
        raw.extend_from_slice(&[0u8; 64]); // invalid signature

        let decoded = decode_transaction(&raw).unwrap();
        let receipt = process_transaction(0, &decoded, &mut host);

        assert!(!receipt.success);
        assert_eq!(receipt.result_code, 8); // ERR_SIG_INVALID
    }

    #[test]
    fn test_process_nonce_mismatch() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_host_with_balance(&sender, 5000);

        // nonce=1 but account nonce is 0
        let raw = encode_transfer_tx(&sender, 1, &to, 1000, &sk);
        let decoded = decode_transaction(&raw).unwrap();

        let receipt = process_transaction(0, &decoded, &mut host);

        assert!(!receipt.success);
    }

    #[test]
    fn test_process_insufficient_balance() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_host_with_balance(&sender, 500);

        let raw = encode_transfer_tx(&sender, 0, &to, 1000, &sk);
        let decoded = decode_transaction(&raw).unwrap();

        let receipt = process_transaction(0, &decoded, &mut host);

        assert!(!receipt.success);
        // Balance should remain unchanged
        assert_eq!(read_balance(&host, &sender).unwrap(), 500);
    }

    #[test]
    fn test_process_out_of_gas() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let ctx = ExecutionContext {
            gas_limit: 100, // too low for sig verification
            ..test_context()
        };
        let mut committed = BTreeMap::new();
        committed.insert(balance_key(&sender), u64_to_le_bytes(5000).to_vec());
        let mut host = MockHost::new(committed, ctx);

        let raw = encode_transfer_tx(&sender, 0, &to, 1000, &sk);
        let decoded = decode_transaction(&raw).unwrap();

        let receipt = process_transaction(0, &decoded, &mut host);

        assert!(!receipt.success);
        assert_eq!(receipt.result_code, 7); // ERR_OUT_OF_GAS
    }

    #[test]
    fn test_sequential_nonces() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_host_with_balance(&sender, 10_000);

        // First tx with nonce 0
        let raw0 = encode_transfer_tx(&sender, 0, &to, 100, &sk);
        let decoded0 = decode_transaction(&raw0).unwrap();
        let receipt0 = process_transaction(0, &decoded0, &mut host);
        assert!(receipt0.success);

        // Second tx with nonce 1
        let raw1 = encode_transfer_tx(&sender, 1, &to, 200, &sk);
        let decoded1 = decode_transaction(&raw1).unwrap();
        let receipt1 = process_transaction(1, &decoded1, &mut host);
        assert!(receipt1.success);

        // Verify cumulative state
        assert_eq!(read_balance(&host, &sender).unwrap(), 9700);
        assert_eq!(read_balance(&host, &to).unwrap(), 300);
        assert_eq!(read_nonce(&host, &sender).unwrap(), 2);
    }

    #[test]
    fn test_balance_key_format() {
        let addr = [0xAB; 32];
        let key = balance_key(&addr);
        assert!(key.starts_with(b"acct/"));
        assert!(key.ends_with(b"/balance"));
    }

    #[test]
    fn test_nonce_key_format() {
        let addr = [0xAB; 32];
        let key = nonce_key(&addr);
        assert!(key.starts_with(b"acct/"));
        assert!(key.ends_with(b"/nonce"));
    }
}
