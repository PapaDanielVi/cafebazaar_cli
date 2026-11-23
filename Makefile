BINARY := bazaar
BIN_DIR := bin
CMD_PKG := ./cmd/bazaar
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Version=$(VERSION) \
	-X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Commit=$(COMMIT) \
	-X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.BuildDate=$(BUILD_DATE)

.PHONY: build test vet install clean

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

install:
	go install -ldflags "$(LDFLAGS)" $(CMD_PKG)

clean:
	rm -rf $(BIN_DIR)
