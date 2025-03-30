package rpc

import (
	"fmt"
	"net/rpc"
	"sync"
)

// Client represents a memory pool client
type Client struct {
	id        int
	client    *rpc.Client
	allocated map[uint64]uint64 // start -> size
	mu        sync.Mutex
}

// NewClient creates a new memory pool client
func NewClient(id int, address string) (*Client, error) {
	client, err := rpc.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err)
	}

	return &Client{
		id:        id,
		client:    client,
		allocated: make(map[uint64]uint64),
	}, nil
}

// Allocate allocates memory through the server
func (c *Client) Allocate(size uint64) (uint64, error) {
	req := &AllocRequest{Size: size}
	resp := &AllocResponse{}

	err := c.client.Call("Server.Allocate", req, resp)
	if err != nil {
		return 0, fmt.Errorf("RPC call failed: %v", err)
	}

	if resp.Error != "" {
		return 0, fmt.Errorf("server error: %s", resp.Error)
	}

	c.mu.Lock()
	c.allocated[resp.Start] = size
	c.mu.Unlock()

	return resp.Start, nil
}

// Free frees memory through the server
func (c *Client) Free(start uint64, size uint64) error {
	req := &FreeRequest{Start: start, Size: size}
	resp := &FreeResponse{}

	err := c.client.Call("Server.Free", req, resp)
	if err != nil {
		return fmt.Errorf("RPC call failed: %v", err)
	}

	if resp.Error != "" {
		return fmt.Errorf("server error: %s", resp.Error)
	}

	c.mu.Lock()
	delete(c.allocated, start)
	c.mu.Unlock()

	return nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.client.Close()
}
