// Package hybrid provides disk space allocation management
package hybrid

import "errors"

// Error definitions
var (
	// ErrSizeTooLarge is returned when requested size exceeds maximum allowed size
	ErrSizeTooLarge = errors.New("requested size is too large")
	// ErrNoSpaceAvailable is returned when no suitable space is available
	ErrNoSpaceAvailable = errors.New("no space available")
	// ErrInvalidAddress is returned when trying to free an invalid address
	ErrInvalidAddress = errors.New("invalid address")
	// ErrSlabNotFound is returned when slab is not found for given address
	ErrSlabNotFound = errors.New("slab not found")
	// ErrSlabFull is returned when slab has no more space
	ErrSlabFull = errors.New("slab is full")
)
