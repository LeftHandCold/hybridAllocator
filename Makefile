.PHONY: all clean test

all: test

test:
	go test -v ./...

clean:
	go clean
	rm -f hsAllocator

run:
	go run main.go 