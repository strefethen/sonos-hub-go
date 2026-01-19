.PHONY: hub build test

hub:
	set -a && source .env && set +a && PORT=9001 air

build:
	go build -o sonos-hub ./cmd/sonos-hub

test:
	go test ./...
