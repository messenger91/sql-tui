BIN := bin/sql-tui

.PHONY: build run test test-race test-integration lint fmt clean db-up db-down

build:
	go build -o $(BIN) ./cmd/sql-tui

run:
	go run ./cmd/sql-tui

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	go test -tags integration ./...

db-up:
	docker compose up -d --wait

db-down:
	docker compose down -v
lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; \
	elif [ -x "$$(go env GOPATH)/bin/staticcheck" ]; then "$$(go env GOPATH)/bin/staticcheck" ./...; \
	else echo "staticcheck not installed; skipped"; fi

fmt:
	gofmt -w .

clean:
	rm -rf bin
