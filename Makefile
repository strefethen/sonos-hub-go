.PHONY: hub build test

hub:
	set -a && source .env && set +a && PORT=9000 air

build:
	go build -o sonos-hub ./cmd/sonos-hub

test:
	go test ./...
