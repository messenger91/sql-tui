BIN := bin/sql-tui

.PHONY: build run test test-race lint fmt clean

build:
	go build -o $(BIN) ./cmd/sql-tui

run:
	go run ./cmd/sql-tui

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	@command -v staticcheck >/dev/null && staticcheck ./... || echo "staticcheck not installed; skipped"

fmt:
	gofmt -w .

clean:
	rm -rf bin
