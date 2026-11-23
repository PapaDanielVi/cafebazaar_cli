BINARY := bazaar
BIN_DIR := bin
CMD_PKG := ./cmd/bazaar
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Version=$(VERSION) \
	-X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Commit=$(COMMIT) \
	-X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.BuildDate=$(BUILD_DATE)

.PHONY: build test vet install clean release-dist

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

install:
	go install -ldflags "$(LDFLAGS)" $(CMD_PKG)

release-dist:
	@rm -rf dist && mkdir -p dist
	@for target in "linux amd64" "linux arm64" "darwin amd64" "darwin arm64" "windows amd64" "windows arm64"; do \
		set -- $$target; goos=$$1; goarch=$$2; \
		stage="dist/stage_$${goos}_$${goarch}"; mkdir -p "$$stage"; \
		bin_name="$(BINARY)"; if [ "$$goos" = "windows" ]; then bin_name="$(BINARY).exe"; fi; \
		CGO_ENABLED=0 GOOS="$$goos" GOARCH="$$goarch" go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o "$${stage}/$${bin_name}" $(CMD_PKG); \
		cp README.md LICENSE "$$stage/"; \
		ver_no_v="$${VERSION#v}"; \
		if [ "$$goos" = "windows" ]; then \
			(cd "$$stage" && zip -q "../$(BINARY)_$${ver_no_v}_$${goos}_$${goarch}.zip" "$$bin_name" README.md LICENSE); \
		else \
			tar -C "$$stage" -czf "dist/$(BINARY)_$${ver_no_v}_$${goos}_$${goarch}.tar.gz" "$$bin_name" README.md LICENSE; \
		fi; \
		rm -rf "$$stage"; \
	done
	@(cd dist && shasum -a 256 $(BINARY)_* > checksums.txt)

clean:
	rm -rf $(BIN_DIR) dist
