.PHONY: build test lint install

build:
	go build -o blaze ./cmd/blaze

test:
	go test -race ./...

lint:
	go vet ./...

install:
	go install ./cmd/blaze
