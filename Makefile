.PHONY: all clean test

all: test

test:
	go test -v ./...

clean:
	go clean

run:
	go run main.go -mode basic

stress10t:
	go run main.go -mode stress10t

stress100t:
	go run main.go -mode stress100t