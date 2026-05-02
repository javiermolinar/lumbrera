.PHONY: build test fmt

build:
	go build ./cmd/lumbrera

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal
