package hybrid

// NewSlab creates a new slab
func NewSlab(start, size uint64, allocator *SlabAllocator, fromBuddy bool) *Slab {
	return &Slab{
		start:     start,
		size:      size,
		used:      0,
		allocator: allocator,
		allocated: make(map[uint64]uint64),
		freeList:  []uint64{start},
		fromBuddy: fromBuddy,
	}
}

// isRangeOverlap checks if the given range overlaps with any allocated range
func (s *Slab) isRangeOverlap(start, size uint64) bool {
	for allocatedStart, allocatedSize := range s.allocated {
		if (start >= allocatedStart && start < allocatedStart+allocatedSize) ||
			(start+size > allocatedStart && start < allocatedStart+allocatedSize) ||
			(start <= allocatedStart && start+size > allocatedStart) {
			return true
		}
	}
	return false
}

// findFreeSpace finds the first available space of the requested size
func (s *Slab) findFreeSpace(size uint64) (uint64, bool) {
	if s.used+size > s.size {
		return 0, false
	}

	for i, freeStart := range s.freeList {
		if freeStart+size <= s.start+s.size {
			if !s.isRangeOverlap(freeStart, size) {
				s.freeList = append(s.freeList[:i], s.freeList[i+1:]...)
				return freeStart, true
			}
		}
	}
	current := s.start
	for current+size <= s.start+s.size {
		if !s.isRangeOverlap(current, size) {
			s.freeList = append(s.freeList, current)
			return current, true
		}
		current += size
	}

	return 0, false
}

// Allocate allocates memory of specified size from slab cache
func (s *SlabAllocator) Allocate(size uint64) (uint64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	Debug("Slab allocating %d bytes", size)
	// Find suitable slab
	slabs, exists := s.cache[size]
	if !exists || len(slabs) == 0 {
		Debug("No existing slab found for size %d, creating new one", size)
		// Get new slab from buddy hybrid
		start, err := s.buddy.Allocate(SlabMaxSize)
		if err != nil {
			Error("Failed to allocate new slab: %v", err)
			return 0, err
		}

		slab := NewSlab(start, SlabMaxSize, s, true)
		s.slabs = append(s.slabs, slab)
		s.cache[size] = []*Slab{slab}
		s.counts[size] = 1
		slabs = s.cache[size]
		Debug("Created new slab at address %d", start)
	}

	// Find slab with available space
	var targetSlab *Slab
	for _, slab := range slabs {
		if slab.used+size <= slab.size {
			targetSlab = slab
			break
		}
	}

	if targetSlab == nil {
		Debug("All existing slabs are full, creating new one")
		// All existing slabs are full, create a new one
		start, err := s.buddy.Allocate(SlabMaxSize)
		if err != nil {
			Error("Failed to allocate new slab: %v", err)
			return 0, err
		}

		targetSlab = NewSlab(start, SlabMaxSize, s, true)
		s.slabs = append(s.slabs, targetSlab)
		s.cache[size] = append(s.cache[size], targetSlab)
		s.counts[size]++
		Debug("Created new slab at address %d", start)
	}

	// Find available space
	start, found := targetSlab.findFreeSpace(size)
	if !found {
		Error("No suitable space found in slab")
		return 0, ErrNoSpaceAvailable
	}

	// Allocate space
	targetSlab.allocated[start] = size
	targetSlab.used += size
	Debug("Allocated %d bytes from slab at address %d", size, start)
	return start, nil
}

// Free releases allocated memory at specified address from slab cache
func (s *SlabAllocator) Free(start uint64) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	Debug("Slab freeing memory at address %d", start)
	// Find target slab
	var targetSlab *Slab
	var targetSize uint64
	for size, slabs := range s.cache {
		for _, slab := range slabs {
			if start >= slab.start && start < slab.start+slab.size {
				targetSlab = slab
				targetSize = size
				break
			}
		}
		if targetSlab != nil {
			break
		}
	}

	if targetSlab == nil {
		Debug("Address not found in slab cache, trying buddy hybrid")
		// Try buddy hybrid if not found in slab cache
		err := s.buddy.Free(start)
		if err != nil {
			Error("Failed to free memory from buddy hybrid: %v", err)
			return err
		}
		return nil
	}

	Debug("Found slab at address %d with size %d", targetSlab.start, targetSize)
	// Calculate block offset
	offset := start - targetSlab.start
	if offset%targetSize != 0 {
		Error("Invalid address %d: offset %d is not aligned with size %d", start, offset, targetSize)
		return ErrInvalidAddress
	}

	// Check if address is actually allocated
	allocatedSize, exists := targetSlab.allocated[start]
	if !exists {
		Error("Address %d is not allocated", start)
		return ErrAddressNotAllocated
	}

	if allocatedSize != targetSize {
		Error("Invalid size for address %d: expected %d, got %d", start, targetSize, allocatedSize)
		return ErrInvalidAddress
	}

	// Update used size and clear allocation record
	targetSlab.used -= targetSize
	delete(targetSlab.allocated, start)
	targetSlab.freeList = append(targetSlab.freeList, start)
	Debug("Updated slab used size to %d", targetSlab.used)

	// If slab is empty and it was allocated from buddy, release it
	if targetSlab.used == 0 && targetSlab.fromBuddy {
		Debug("Slab is empty and was allocated from buddy, releasing it")
		// Remove from slabs list
		for i, slab := range s.slabs {
			if slab == targetSlab {
				s.slabs = append(s.slabs[:i], s.slabs[i+1:]...)
				break
			}
		}

		// Remove from cache
		slabs := s.cache[targetSize]
		for i, slab := range slabs {
			if slab == targetSlab {
				if len(slabs) == 1 {
					delete(s.cache, targetSize)
					delete(s.counts, targetSize)
				} else {
					s.cache[targetSize] = append(slabs[:i], slabs[i+1:]...)
					s.counts[targetSize]--
				}
				break
			}
		}

		err := s.buddy.Free(targetSlab.start)
		if err != nil {
			Error("Failed to free empty slab: %v", err)
			return err
		}
		Debug("Successfully released empty slab")
	}

	return nil
}

// GetUsedSize returns the total size of allocated memory from slab cache
func (s *SlabAllocator) GetUsedSize() uint64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var used uint64
	for _, slab := range s.slabs {
		used += slab.used
	}
	Debug("Slab hybrid used size: %d bytes", used)
	return used
}
