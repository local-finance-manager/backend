BINARY    = bin/server
MAIN      = ./cmd/server
MODULE    = github.com/local-finance-manager/backend

.PHONY: all setup build run dev test cover lint clean

all: build

## setup: download and tidy dependencies
setup:
	go mod tidy

## build: compile the binary
build:
	@mkdir -p bin
	go build -ldflags="-s -w" -o $(BINARY) $(MAIN)

## run: build and run locally
run: build
	./$(BINARY)

## dev: hot reload with air (requires: go install github.com/air-verse/air@latest)
dev:
	air

## test: run all tests with race detector
test:
	go test -race -cover ./...

## cover: run tests with coverage report; fails if total coverage < 85%
cover:
	@go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out
	@TOTAL=$$(go tool cover -func=coverage.out | grep "^total:" | awk '{print $$3}' | tr -d '%'); \
	  echo "Total coverage: $${TOTAL}%"; \
	  if [ "$$(echo "$${TOTAL} < 85" | bc)" -eq 1 ]; then \
	    echo "FAIL: coverage $${TOTAL}% below 85% minimum"; exit 1; \
	  fi; \
	  echo "PASS: coverage $${TOTAL}% meets 85% threshold"

## lint: run linter (requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint:
	golangci-lint run ./...

## clean: remove build artifacts
clean:
	@rm -rf bin/ tmp/

## help: print this message
help:
	@grep -E '^## ' Makefile | sed 's/## //'
