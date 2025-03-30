.PHONY: all clean test

all: test

test:
	go test -v ./...

clean:
	go clean

run:
	go run main.go 