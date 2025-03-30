package rpc

import (
	"testing"
	"time"
)

const (
	ServerAddress = "localhost:1234"
)

func TestRPCClientServer(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	go func() {
		if err := server.Start(ServerAddress); err != nil {
			t.Errorf("Server error: %v", err)
		}
	}()

	time.Sleep(time.Second)

	numClients := 5
	clients := make([]*Client, numClients)

	for i := 0; i < numClients; i++ {
		client, err := NewClient(i, ServerAddress)
		if err != nil {
			t.Fatalf("Failed to create client %d: %v", i, err)
		}
		clients[i] = client
		defer client.Close()
	}

	done := make(chan bool)
	for i, client := range clients {
		go func(id int, c *Client) {
			start, err := c.Allocate(1024 * 1024) // 1MB
			if err != nil {
				t.Errorf("Client %d allocation failed: %v", id, err)
				done <- true
				return
			}

			time.Sleep(time.Millisecond * 100)

			if err := c.Free(start, 1024*1024); err != nil {
				t.Errorf("Client %d free failed: %v", id, err)
			}

			done <- true
		}(i, client)
	}

	for i := 0; i < numClients; i++ {
		<-done
	}

	server.Close()
}
