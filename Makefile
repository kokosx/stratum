.PHONY: generate migrate seed build build-freebsd build-freebsd-arm64 test run fmt fmt-check check

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

check: fmt-check
	go vet ./...
	go test ./...
	go build ./...

generate:
	sqlc generate

migrate:
	go run ./cmd/stratum migrate

seed:
	go run ./cmd/stratum seed

build:
	go build -ldflags "-X github.com/kokosx/stratum/internal/version.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" ./cmd/stratum

build-freebsd:
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o stratum-freebsd-amd64 ./cmd/stratum

build-freebsd-arm64:
	GOOS=freebsd GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o stratum-freebsd-arm64 ./cmd/stratum

test:
	go test ./...

run: build
	./stratum
