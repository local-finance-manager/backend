BINARY    = bin/server
MAIN      = ./cmd/server
MODULE    = github.com/local-finance-manager/backend

# Pacotes de negócio medidos pela cobertura. Excluídos por natureza (não unit-testáveis):
#   cmd/server (Composition Root / wiring) e database/config/middleware (sem lógica de
#   negócio). O backup é coberto inclusive no adapter do Drive (mock httptest) e no fluxo
#   OAuth Authorize (loopback + endpoint de token mockado).
COVER_PKGS = ./internal/backup/... ./internal/budget/... ./internal/category/... ./internal/creditcard/... \
             ./internal/installment/... ./internal/patrimonio/... ./internal/report/... ./internal/shared/... ./internal/transaction/...

# Piso de cobertura exigido por módulo.
COVER_MIN = 90

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

## cover: run tests with coverage; fails if ANY module is below COVER_MIN% (per módulo)
cover:
	@echo "Cobertura por módulo (mínimo $(COVER_MIN)%):"; \
	fail=0; \
	for pkg in $(COVER_PKGS); do \
	  pct=$$(go test -race -covermode=atomic -cover $$pkg 2>/dev/null | grep -o 'coverage: [0-9.]*%' | grep -o '[0-9.]*'); \
	  printf "  %-32s %6s%%\n" "$$pkg" "$$pct"; \
	  if [ -z "$$pct" ] || [ "$$(echo "$$pct < $(COVER_MIN)" | bc)" -eq 1 ]; then \
	    echo "  FAIL: $$pkg abaixo de $(COVER_MIN)%"; fail=1; \
	  fi; \
	done; \
	if [ $$fail -eq 1 ]; then exit 1; fi; \
	echo "PASS: todos os módulos >= $(COVER_MIN)%"

## lint: run linter (requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint:
	golangci-lint run ./...

## clean: remove build artifacts
clean:
	@rm -rf bin/ tmp/

## help: print this message
help:
	@grep -E '^## ' Makefile | sed 's/## //'
