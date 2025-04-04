// Package hybrid provides disk space allocation management
package hybrid

import "unsafe"

// NewAllocator creates a new memory hybrid instance
func NewAllocator() *Allocator {
	Debug("Creating new hybrid")
	buddy := NewBuddyAllocator()

	slab := NewSlabAllocator(buddy)
	allocator := &Allocator{
		buddy: buddy,
		slab:  slab,
	}
	return allocator
}

// Allocate allocates memory of specified size
func (a *Allocator) Allocate(size uint64) (uint64, error) {
	Debug("Allocating %d bytes", size)
	if size > MaxBlockSize {
		Error("Requested size %d exceeds MaxBlockSize %d", size, MaxBlockSize)
		return 0, ErrSizeTooLarge
	}

	if size <= SlabMaxSize {
		start, err := a.slab.Allocate(size)
		if err == ErrSlabFull {
			Debug("Slab is full, trying buddy hybrid")
			return a.buddy.Allocate(size)
		}
		if err != nil {
			return 0, err
		}
		Debug("Allocated %d bytes from slab at address %d", size, start)
		return start, nil
	}

	start, err := a.buddy.Allocate(size)
	if err != nil {
		return 0, err
	}
	Debug("Allocated %d bytes from buddy at address %d", size, start)
	return start, nil
}

// Free releases allocated memory at specified address
func (a *Allocator) Free(start uint64, size uint64) error {
	Debug("Freeing %d bytes at address %d", size, start)
	if size <= SlabMaxSize {
		err := a.slab.Free(start, size)
		if err == ErrSlabNotFound {
			Debug("Address not found in slab, trying buddy hybrid")
			return a.buddy.Free(start)
		}
		if err != nil {
			Error("Slab free failed: %v", err)
			return err
		}
		Debug("Freed %d bytes from slab at address %d", size, start)
		return nil
	}

	err := a.buddy.Free(start)
	if err != nil {
		Error("Buddy free failed: %v", err)
		return err
	}
	Debug("Freed %d bytes from buddy at address %d", size, start)
	return nil
}

// GetUsedSize returns the total size of allocated memory
func (a *Allocator) GetUsedSize() uint64 {
	used := a.buddy.GetUsedSize() - a.slab.GetFreeSize()
	Debug("Total used size: %d bytes", used)
	return used
}

func (a *Allocator) GetTotalSize() uint64 {
	base := 20
	offset := MaxOrder - base

	return 1 << (40 + uint(offset))
}

// GetMemoryUsage returns the memory overhead of the hybrid
func (a *Allocator) GetMemoryUsage() uint64 {
	var size uint64
	// Calculate buddy allocator memory usage
	size = a.buddy.GetMemoryUsage()

	// Calculate slab allocator memory usage
	size += uint64(unsafe.Sizeof(&Slab{})) * uint64(len(a.slab.cache))
	for _, slabs := range a.slab.cache {
		size += uint64(unsafe.Sizeof(slabs)) + uint64(len(slabs))*uint64(unsafe.Sizeof(&Slab{}))
	}

	Debug("Memory overhead: %d bytes", size)
	return size
}

func (a *Allocator) Close() error {
	a.buddy.Close()
	a.slab.Close()
	return nil
}
