.PHONY: generate migrate seed build test run

generate:
	sqlc generate

migrate:
	go run ./cmd/stratum migrate

seed:
	go run ./cmd/stratum seed

build:
	go build ./cmd/stratum

test:
	go test ./...

run: build
	./stratum
