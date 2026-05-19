.PHONY: build test fmt goreleaser-check release-snapshot

build:
	go build ./cmd/lumbrera

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

goreleaser-check:
	goreleaser check

release-snapshot:
	goreleaser release --snapshot --clean
