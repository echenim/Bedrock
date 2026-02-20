//! Safe WASM linear memory read/write helpers with bounds checking.
//!
//! All functions validate pointer and length arguments against the guest's
//! linear memory size before accessing. Out-of-bounds access returns
//! `ERR_BAD_POINTER` (EXECUTION_SPEC.md §5.1).

use bedrock_hostapi::HostError;
use bedrock_primitives::ErrorCode;

/// Read `len` bytes from guest memory at `ptr`.
///
/// Returns `Err(BadPointer)` if the range `[ptr, ptr+len)` is out of bounds.
pub fn read_bytes(mem: &[u8], ptr: i32, len: i32) -> Result<Vec<u8>, HostError> {
    if ptr < 0 || len < 0 {
        return Err(HostError::bad_pointer());
    }
    let start = ptr as usize;
    let end = start
        .checked_add(len as usize)
        .ok_or_else(HostError::bad_pointer)?;
    if end > mem.len() {
        return Err(HostError::bad_pointer());
    }
    Ok(mem[start..end].to_vec())
}

/// Write `data` bytes to guest memory at `ptr`.
///
/// Returns `Err(BadPointer)` if the range `[ptr, ptr+data.len())` is out of bounds.
pub fn write_bytes(mem: &mut [u8], ptr: i32, data: &[u8]) -> Result<(), HostError> {
    if ptr < 0 {
        return Err(HostError::bad_pointer());
    }
    let start = ptr as usize;
    let end = start
        .checked_add(data.len())
        .ok_or_else(HostError::bad_pointer)?;
    if end > mem.len() {
        return Err(HostError::bad_pointer());
    }
    mem[start..end].copy_from_slice(data);
    Ok(())
}

/// Read an i32 value (little-endian) from guest memory at `ptr`.
pub fn read_i32(mem: &[u8], ptr: i32) -> Result<i32, HostError> {
    let bytes = read_bytes(mem, ptr, 4)?;
    Ok(i32::from_le_bytes([bytes[0], bytes[1], bytes[2], bytes[3]]))
}

/// Write an i32 value (little-endian) to guest memory at `ptr`.
pub fn write_i32(mem: &mut [u8], ptr: i32, value: i32) -> Result<(), HostError> {
    write_bytes(mem, ptr, &value.to_le_bytes())
}

/// Validate that a pointer range `[ptr, ptr+len)` is within memory bounds.
/// Returns the error code directly for use in linker functions.
pub fn validate_range(mem_size: usize, ptr: i32, len: i32) -> Result<(), i32> {
    if ptr < 0 || len < 0 {
        return Err(ErrorCode::BadPointer as i32);
    }
    let end = (ptr as usize)
        .checked_add(len as usize)
        .ok_or(ErrorCode::BadPointer as i32)?;
    if end > mem_size {
        return Err(ErrorCode::BadPointer as i32);
    }
    Ok(())
}

/// Compute how many 8-byte-aligned bytes are needed.
fn align8(size: usize) -> usize {
    (size + 7) & !7
}

/// Parameters for the host-side bump allocator within guest memory.
///
/// After module instantiation, the sandbox grows memory to reserve a region
/// for host-allocated buffers. Allocations are bump-pointer style — no
/// deallocation (the entire instance is short-lived).
#[derive(Debug, Clone)]
pub struct HostAllocator {
    /// Base address of the host allocation region in guest memory.
    pub base: usize,
    /// Current bump offset from base.
    pub bump: usize,
    /// Total bytes available in the allocation region.
    pub capacity: usize,
}

impl HostAllocator {
    /// Create a new allocator for a region starting at `base` with `capacity` bytes.
    pub fn new(base: usize, capacity: usize) -> Self {
        Self {
            base,
            bump: 0,
            capacity,
        }
    }

    /// Compute an allocation. Returns `(ptr, new_bump, new_capacity, grow_pages)`.
    ///
    /// If the current region is full, `grow_pages > 0` indicates memory must
    /// be grown before writing. The caller is responsible for actually growing
    /// memory and then writing data at `ptr`.
    pub fn compute_alloc(&self, size: usize) -> (usize, usize, usize, u64) {
        let aligned = align8(size.max(1));
        if self.bump + aligned <= self.capacity {
            let ptr = self.base + self.bump;
            (ptr, self.bump + aligned, self.capacity, 0)
        } else {
            let deficit = self.bump + aligned - self.capacity;
            let extra_pages = deficit.div_ceil(65536) as u64;
            let new_capacity = self.capacity + (extra_pages as usize) * 65536;
            let ptr = self.base + self.bump;
            (ptr, self.bump + aligned, new_capacity, extra_pages)
        }
    }

    /// Update allocator state after a successful allocation.
    pub fn commit(&mut self, new_bump: usize, new_capacity: usize) {
        self.bump = new_bump;
        self.capacity = new_capacity;
    }
}

/// Initial host allocation region size in pages (4 pages = 256 KiB).
pub const HOST_ALLOC_PAGES: u64 = 4;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_read_bytes_basic() {
        let mem = vec![10, 20, 30, 40, 50];
        let result = read_bytes(&mem, 1, 3).unwrap();
        assert_eq!(result, vec![20, 30, 40]);
    }

    #[test]
    fn test_read_bytes_out_of_bounds() {
        let mem = vec![10, 20, 30];
        assert!(read_bytes(&mem, 1, 3).is_err());
        assert!(read_bytes(&mem, -1, 1).is_err());
        assert!(read_bytes(&mem, 0, -1).is_err());
    }

    #[test]
    fn test_write_bytes_basic() {
        let mut mem = vec![0; 8];
        write_bytes(&mut mem, 2, &[0xAA, 0xBB]).unwrap();
        assert_eq!(mem[2], 0xAA);
        assert_eq!(mem[3], 0xBB);
    }

    #[test]
    fn test_write_bytes_out_of_bounds() {
        let mut mem = vec![0; 4];
        assert!(write_bytes(&mut mem, 2, &[1, 2, 3]).is_err());
    }

    #[test]
    fn test_read_write_i32() {
        let mut mem = vec![0; 16];
        write_i32(&mut mem, 4, 0x12345678).unwrap();
        assert_eq!(read_i32(&mem, 4).unwrap(), 0x12345678);
    }

    #[test]
    fn test_validate_range() {
        assert!(validate_range(100, 0, 100).is_ok());
        assert!(validate_range(100, 0, 101).is_err());
        assert!(validate_range(100, -1, 1).is_err());
        assert!(validate_range(100, 50, -1).is_err());
    }

    #[test]
    fn test_host_allocator_basic() {
        let alloc = HostAllocator::new(65536, 65536 * 4);
        let (ptr, new_bump, new_cap, grow) = alloc.compute_alloc(100);
        assert_eq!(ptr, 65536);
        assert_eq!(new_bump, 104); // aligned to 8
        assert_eq!(new_cap, 65536 * 4);
        assert_eq!(grow, 0);
    }

    #[test]
    fn test_host_allocator_needs_grow() {
        let alloc = HostAllocator::new(65536, 64); // only 64 bytes available
        let (ptr, new_bump, new_cap, grow) = alloc.compute_alloc(100);
        assert_eq!(ptr, 65536);
        assert!(grow > 0);
        assert!(new_cap >= new_bump);
    }

    #[test]
    fn test_host_allocator_sequential() {
        let mut alloc = HostAllocator::new(1000, 1000);
        let (ptr1, bump1, cap1, _) = alloc.compute_alloc(10);
        alloc.commit(bump1, cap1);
        let (ptr2, bump2, cap2, _) = alloc.compute_alloc(20);
        alloc.commit(bump2, cap2);

        assert_eq!(ptr1, 1000);
        assert_eq!(ptr2, 1000 + 16); // 10 aligned to 16
        assert!(ptr2 > ptr1);
    }
}
