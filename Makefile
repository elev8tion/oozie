.PHONY: run test build

run:
	go run ./cmd/app

test:
	go test ./...

build:
	go build ./cmd/app
