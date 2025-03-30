package hsAllocator

import (
	"math"
)

// NewBuddyAllocator creates a new buddy allocator
func NewBuddyAllocator() *BuddyAllocator {
	return &BuddyAllocator{
		blocks:    [21][]*Block{},
		allocated: make(map[uint64]*Block),
	}
}

// getOrder calculates the order value for a given size
func getOrder(size uint64) int {
	if size < MinBlockSize {
		return 0
	}
	size = (size + MinBlockSize - 1) & ^uint64(MinBlockSize-1) // Round up to nearest MinBlockSize
	order := int(math.Log2(float64(size) / float64(MinBlockSize)))
	Debug("Calculated order %d for size %d", order, size)
	return order
}

// Allocate allocates memory of specified size
func (b *BuddyAllocator) Allocate(size uint64) (uint64, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	order := getOrder(size)
	if order > MaxOrder {
		Error("Order %d exceeds MaxOrder %d", order, MaxOrder)
		return 0, ErrSizeTooLarge
	}

	Debug("Looking for block of order %d", order)
	// Find available block from current order up
	for i := order; i <= MaxOrder; i++ {
		if len(b.blocks[i]) > 0 {
			block := b.blocks[i][0]
			b.blocks[i] = b.blocks[i][1:]

			// Split block if too large
			if i > order {
				Debug("Splitting block of order %d into smaller blocks", i)
				for j := i - 1; j >= order; j-- {
					newBlock := &Block{
						start:  block.start + (1<<uint(j))*MinBlockSize,
						size:   (1 << uint(j)) * MinBlockSize,
						isFree: true,
					}
					block.size = (1 << uint(j)) * MinBlockSize
					b.blocks[j] = append(b.blocks[j], newBlock)
					Debug("Created new block of order %d at address %d", j, newBlock.start)
				}
			}

			block.isFree = false
			b.allocated[block.start] = block // 记录已分配的块
			Debug("Allocated block of order %d at address %d, size %d", order, block.start, block.size)
			return block.start, nil
		}
	}

	Error("No space available for allocation of size %d", size)
	return 0, ErrNoSpaceAvailable
}

// Free releases allocated memory at specified address
func (b *BuddyAllocator) Free(start uint64) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	Debug("Attempting to free block at address %d", start)

	block, exists := b.allocated[start]
	if !exists {
		Error("Invalid address %d: block not found in allocated list", start)
		return ErrInvalidAddress
	}

	order := getOrder(block.size)
	Debug("Found block of order %d at address %d", order, start)

	delete(b.allocated, start)

	// Try to merge with buddy blocks
	for {
		buddyStart := start ^ (1 << uint(order) * MinBlockSize)
		Debug("Looking for buddy block at address %d", buddyStart)
		var buddyIndex int = -1
		for i, buddyBlock := range b.blocks[order] {
			if buddyBlock.start == buddyStart && buddyBlock.isFree {
				buddyIndex = i
				break
			}
		}

		if buddyIndex == -1 {
			Debug("No buddy block found, adding block as free")
			// Add current block as free
			newBlock := &Block{
				start:  start,
				size:   (1 << uint(order)) * MinBlockSize,
				isFree: true,
			}
			b.blocks[order] = append(b.blocks[order], newBlock)
			break
		}

		Debug("Found buddy block, merging blocks")
		// Remove buddy block
		b.blocks[order] = append(b.blocks[order][:buddyIndex], b.blocks[order][buddyIndex+1:]...)

		// Merge blocks
		if start > buddyStart {
			start = buddyStart
		}
		order++
		if order > MaxOrder {
			break
		}
	}

	Debug("Successfully freed block at address %d", start)
	return nil
}

// GetUsedSize returns the total size of allocated memory
func (b *BuddyAllocator) GetUsedSize() uint64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	var used uint64
	for _, block := range b.allocated {
		used += block.size
	}
	Debug("Buddy allocator used size: %d bytes", used)
	return used
}
