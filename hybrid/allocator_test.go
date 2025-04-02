package hybrid

import (
	"fmt"
	"strings"
	"testing"
)

const (
	MB = 1024 * 1024
	KB = 1024
)

func TestAllocator(t *testing.T) {
	// Create a new hybrid for testing
	allocator := NewAllocator()

	// Test basic allocation and free
	t.Run("Basic allocation and free", func(t *testing.T) {
		size := uint64(4 * 1024)
		start, err := allocator.Allocate(size)
		if err != nil {
			t.Fatalf("Failed to allocate 4KB: %v", err)
		}
		err = allocator.Free(start, size)
		if err != nil {
			t.Fatalf("Failed to free allocated space: %v", err)
		}
	})

	// Test large block allocation
	t.Run("Large block allocation", func(t *testing.T) {
		size := uint64(2 * 1024 * 1024)
		start, err := allocator.Allocate(size)
		if err != nil {
			t.Fatalf("Failed to allocate 2MB: %v", err)
		}

		err = allocator.Free(start, size)
		if err != nil {
			t.Fatalf("Failed to free allocated space: %v", err)
		}
	})

	// Test multiple allocations
	t.Run("Multiple allocations", func(t *testing.T) {
		// Allocate multiple small blocks
		addresses := make([]uint64, 10)
		size := uint64(4 * 1024)
		for i := 0; i < 10; i++ {
			start, err := allocator.Allocate(size)
			if err != nil {
				t.Fatalf("Failed to allocate 4KB: %v", err)
			}
			addresses[i] = start
		}

		// Free all allocated space
		for _, start := range addresses {
			err := allocator.Free(start, size)
			if err != nil {
				t.Fatalf("Failed to free allocated space: %v", err)
			}
		}
	})

	// Test invalid free
	t.Run("Invalid free", func(t *testing.T) {
		err := allocator.Free(0xdeadbeef, 4096)
		if err != ErrInvalidAddress &&
			err != ErrBlockNotFound {
			t.Errorf("Expected ErrInvalidAddress, got %v", err)
		}
	})

	// Test space utilization
	t.Run("Space utilization", func(t *testing.T) {
		// Allocate multiple blocks of different sizes
		addresses := make([]uint64, 0)
		sizes := []uint64{4 * 1024, 8 * 1024, 16 * 1024, 32 * 1024, 64 * 1024}
		for _, size := range sizes {
			start, err := allocator.Allocate(size)
			if err != nil {
				t.Fatalf("Failed to allocate %d bytes: %v", size, err)
			}
			addresses = append(addresses, start)
		}

		// Check usage
		used := allocator.GetUsedSize()
		utilization := float64(used) / float64(MaxBlockSize) * 100
		t.Logf("Space utilization: %.5f%%", utilization)

		// Free all space
		for i, start := range addresses {
			err := allocator.Free(start, sizes[i])
			if err != nil {
				t.Fatalf("Failed to free allocated space: %v", err)
			}
		}
	})
}

func BenchmarkAlloc(b *testing.B) {
	sizes := []uint64{
		4 * KB,
		16 * KB,
		64 * KB,
		256 * KB,
		1 * MB,
		4 * MB,
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%dKB", size/KB), func(b *testing.B) {
			allocator := NewAllocator()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := allocator.Allocate(size)
				if err != nil {
					if strings.Contains(err.Error(), "no space available") {
						break
					}
					b.Fatalf("Failed to allocate %d bytes: %v", size, err)
				}
			}
			b.StopTimer()
		})
	}
}
