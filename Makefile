BINARY := bazaar
BIN_DIR := bin
CMD_PKG := ./cmd/bazaar
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Version=$(VERSION) \
	-X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Commit=$(COMMIT) \
	-X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.BuildDate=$(BUILD_DATE)

.PHONY: build test vet lint security install clean release-dist demos

DEMO_DIR ?= .
DEMO_MASK_PATH ?= ~/projects/cafebazaar-sample-app

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

security:
	govulncheck ./...
	golangci-lint run --enable-only=gosec ./...

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

demos:
	@command -v vhs >/dev/null 2>&1 || { echo "vhs is required (https://github.com/charmbracelet/vhs)"; exit 1; }
	@REPO_DIR="$$(pwd)"; \
	TARGET_DIR="$$(cd "$(DEMO_DIR)" && pwd)"; \
	TMP_DIR="$$(mktemp -d)"; \
	trap 'rm -rf "$$TMP_DIR"' EXIT; \
	mkdir -p "$$TMP_DIR/bin"; \
	REAL_BAZAAR="$$(command -v bazaar || echo "$$REPO_DIR/$(BIN_DIR)/$(BINARY)")"; \
	printf '%s\n' \
		'#!/usr/bin/env python3' \
		'import codecs, fcntl, os, pty, select, subprocess, sys, termios' \
		'real_bin = os.environ["BAZAAR_REAL_BIN"]' \
		'target = os.environ["BAZAAR_DEMO_TARGET"]' \
		'home = os.path.expanduser("~")' \
		'short_target = "~" + target[len(home):] if target.startswith(home + "/") else target' \
		'mask = os.environ["BAZAAR_DEMO_MASK"]' \
		'master, slave = pty.openpty()' \
		'try:' \
		'    ws = fcntl.ioctl(sys.stdout.fileno(), termios.TIOCGWINSZ, b"\x00" * 8)' \
		'    fcntl.ioctl(slave, termios.TIOCSWINSZ, ws)' \
		'except Exception:' \
		'    pass' \
		'proc = subprocess.Popen([real_bin] + sys.argv[1:], stdin=slave, stdout=slave, stderr=slave, close_fds=True)' \
		'os.close(slave)' \
		'decoder = codecs.getincrementaldecoder("utf-8")("replace")' \
		'buf = ""' \
		'while True:' \
		'    r, _, _ = select.select([master], [], [], 0.1)' \
		'    if r:' \
		'        try:' \
		'            data = os.read(master, 4096)' \
		'        except OSError:' \
		'            break' \
		'        if not data:' \
		'            break' \
		'        buf += decoder.decode(data, final=False)' \
		'        while "\n" in buf:' \
		'            line, buf = buf.split("\n", 1)' \
		'            line = line.replace(short_target, mask).replace(target, mask).replace(home, "~")' \
		'            sys.stdout.buffer.write((line + "\n").encode("utf-8"))' \
		'            sys.stdout.buffer.flush()' \
		'    elif proc.poll() is not None:' \
		'        break' \
		'buf += decoder.decode(b"", final=True)' \
		'if buf:' \
		'    buf = buf.replace(short_target, mask).replace(target, mask).replace(home, "~")' \
		'    sys.stdout.buffer.write(buf.encode("utf-8"))' \
		'    sys.stdout.buffer.flush()' \
		'os.close(master)' \
		'sys.exit(proc.wait())' > "$$TMP_DIR/bin/bazaar"; \
	chmod +x "$$TMP_DIR/bin/bazaar"; \
	for tape in android-gradle status-and-config ci-and-cli-tools; do \
		sed "s|Output \"assets/demos/|Output \"$$REPO_DIR/assets/demos/|g" "$$REPO_DIR/assets/demos/$${tape}.tape" > "$$TMP_DIR/$${tape}.tape"; \
		if [ "$$tape" != "ci-and-cli-tools" ]; then \
			python3 -c 'import pathlib,sys; p=pathlib.Path(sys.argv[1]); s=p.read_text(); inj=f"Type '\''export PATH=\"{sys.argv[2]}/bin:$$PATH\"'\''\nEnter\n"; p.write_text(s.replace("Type \"clear\"\nEnter\nSleep 400ms\nShow", inj+"Type \"clear\"\nEnter\nSleep 400ms\nShow", 1))' "$$TMP_DIR/$${tape}.tape" "$$TMP_DIR"; \
		fi; \
		(cd "$$TARGET_DIR" && BAZAAR_REAL_BIN="$$REAL_BAZAAR" BAZAAR_DEMO_TARGET="$$TARGET_DIR" BAZAAR_DEMO_MASK="$(DEMO_MASK_PATH)" vhs "$$TMP_DIR/$${tape}.tape"); \
	done

clean:
	rm -rf $(BIN_DIR) dist
