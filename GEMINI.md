# GEMINI.md

Project instructions and architectural reference for `bazaar` (`github.com/PapaDanielVi/cafebazaar_cli`), a Go CLI for CafeBazaar Android app publishing, release management, user reviews, and Android Gradle workflows.

## Architecture

- `cmd/bazaar/main.go`: CLI entry point wiring signal-aware `context.Context` and deterministic process exit codes.
- `internal/api/httpclient`: shared HTTP transport with UTF-8 JSON encoding (`SetEscapeHTML(false)`), streaming multipart uploads with progress callbacks, HTTP `429` retry/backoff, configurable per-request timeouts (`DefaultRequestTimeout = 2m`, `DefaultPublishTimeout = 10m` via `DoJSONWithTimeout`, `DefaultUploadTimeout = 15m`), client identification headers (`app_name=cli`, `version=<cli version>`), HTTP/HTTPS/SOCKS5 proxy support (`ParseProxyURL`, `MaskProxyURL`), and unified error parsing for Pishkhan (`type`/`message`) and Sandoghche (`detail`).
- `internal/api/pishkhan`: client for all 10 Pishkhan Release API endpoints on `https://api.pishkhan.cafebazaar.ir` plus `WaitForBundle` polling (`U`/`S`/`P`/`D`/`E`). `CommitRelease` uses `DefaultPublishTimeout` (10 minutes) because CafeBazaar can take several minutes to inspect APKs and publish.
- `internal/api/comments`: client for both Sandoghche Comments API endpoints on `https://sandoghche.cafebazaar.ir` (`GET /api/v0/reviews/` with all 7 filters and cursor pagination, `POST /api/v0/reviews/{id}/`).
- `internal/api/publisher`: extensible developer-account Nasher Publisher API client (`POST /v1/apps/` with `issue_api_agent` token extraction, warning handling, and 10m/15m timeouts).
- `internal/auth`: `SecretStore` using native OS keychain (`go-keyring`, service `bazaar-cli`) with automatic atomic `0600` file fallback (`~/.config/bazaar/credentials.yaml`).
- `internal/config`: global config (`~/.config/bazaar/config.yaml`), non-secret project config (`.bazaar.yaml`), environment variable parsing (`env.go`), and folder-to-secret app resolution (`resolver.go`).
- `internal/android`: pure-Go Gradle Groovy/Kotlin DSL parser (`build.gradle`, `build.gradle.kts`, `gradle.properties`, `gradle/libs.versions.toml`), `buildTypes` x `productFlavors` variant synthesis, in-place `versionCode`/`versionName` bumper, APK/AAB/`.bin` discovery, and `./gradlew` runner.
- `internal/gitflow`: thin `git` CLI wrapper via `exec.CommandContext` for status, commits-since-tag changelog drafts, file-scoped commits, annotated tags, and opt-in pushes.
- `internal/output`: Charmbracelet (`lipgloss` + `huh`) terminal UI, pure-Go Persian (Farsi) glyph shaper and BiDi formatter (`persian.go`, including `SanitizeInline`, `WrapRunes`, `AnchorBiDiLine`), interactive vs headless CI mode detection (`mode.go`), upload progress bar with percentage/speed/ETA and post-100% server-processing spinner (`progress.go`), elapsed-time Braille spinner, and centralized `ConfirmDangerousAction` guard (`confirm.go`).
- `internal/publish`: end-to-end Android release pipeline orchestrator (`--dry-run` safe).
- `internal/cli`: Cobra command tree (`auth`, `init`, `status`, `android`, `release`, `publish`, `review`, `app`, `version`, `completion`), dynamic shell completions (`completions.go`), and tilde/relative path display helpers (`shortenDisplayPath`, `relativeDisplayPath`).
- `scripts/install.sh` and `scripts/install.ps1`: prebuilt release binary installers for Linux, macOS, and Windows.
- `Dockerfile` and `.github/workflows/docker.yml`: multi-arch (`linux/amd64`, `linux/arm64`) Alpine container images published to `ghcr.io/papadanielvi/cafebazaar_cli`.
- `assets/demos/`: portable Charmbracelet VHS `.tape` scripts and rendered `.gif` demos (regenerated with `make demos DEMO_DIR=...`).

## Key Invariants

1. **Two-token model**:
   - Developer Nasher token (`publisher:default`, `CAFEBAZAAR_PUBLISHER_TOKEN`) is only for creating new apps (`POST /v1/apps/`).
   - Per-app Pishkhan token (`app:<package_name>`, `CAFEBAZAAR_APP_TOKEN`) is sent in `CAFEBAZAAR-PISHKHAN-API-SECRET` for both Pishkhan Release API and Sandoghche Comments API.
   - Never write raw tokens into `.bazaar.yaml` or inside a user's Git repository.
2. **App resolution precedence**: `CAFEBAZAAR_APP_TOKEN` > `--app` / `CAFEBAZAAR_APP` > `.bazaar.yaml` > bound folder path in `~/.config/bazaar/config.yaml` > auto-detected Gradle package match > `default_app`.
3. **Proxy precedence**: `--proxy` > `BAZAAR_PROXY` / `CAFEBAZAAR_PROXY` > `.bazaar.yaml` `proxy` > global `config.yaml` `proxy` > standard `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`. Use `direct` or `none` to bypass system env proxies. Always mask proxy credentials before logging.
4. **Confirmation safety**: every destructive or externally visible mutation (`release delete`, `release commit`, `publish`, `review reply`, `app create`, token overwrite/removal, Git push) must call `Printer.ConfirmDangerousAction`. Interactive prompts default to No; non-interactive/CI runs refuse with `ExitConfirmation` (code `4`) unless `--yes` or `CAFEBAZAAR_YES=1` is set.
5. **Persian (Farsi) UTF-8 handling**: never slice strings by byte index (use `TruncateRunes` and `WrapRunes`). Keep HTTP JSON, `--output json`, `--output yaml`, and non-TTY piped output in unmodified logical UTF-8 (`SetEscapeHTML(false)`). Inside closed 4-sided cards, keep Persian lines in regular weight (not bold) and use `AnchorBiDiLine` so right borders stay aligned in native BiDi terminals; only apply glyph shaping/BiDi when `RuntimeMode.RTL` is true, preserving ANSI escape sequences.
6. **Upload and publish timeouts**: keep `DefaultPublishTimeout` at 10 minutes (`CommitRelease`, JSON `CreateApp`) and `DefaultUploadTimeout` at 15 minutes (`UploadAPK`, `UploadAAB`, `UploadBin`, multipart `CreateApp`). During interactive uploads, display percentage, speed, and remaining ETA, then switch to a server-processing spinner at 100% until CafeBazaar responds.
7. **Headless CI mode**: `--no-ui`, `--plain`, `CI=true`, `BAZAAR_NO_UI=1`, `BAZAAR_PLAIN=1`, or non-TTY IO must never emit ANSI escapes, spinners, or interactive prompts.

## Go Coding Standards

- Eliminate duplicate literals; define constants for values used more than once.
- Accept `context.Context` as the first parameter on every custom function and method.
- Write all `.go` comments as full sentences ending with a period.
- Use external test packages (`package <name>_test`) and call `t.Parallel()` in table-driven tests where safe.
- Use `errors.New` for static errors and `fmt.Errorf` only when formatting is needed.
- Never ignore returned errors; check and handle every error return.

## Commands

```bash
make vet                           # go vet ./...
make lint                          # golangci-lint run ./...
make security                      # govulncheck ./... and gosec checks
make test                          # go test -race -count=1 ./...
make build                         # builds bin/bazaar
make install                       # installs bazaar into $GOBIN
make release-dist                  # cross-compiles linux/darwin/windows (amd64/arm64) archives + checksums.txt
make demos DEMO_DIR=/path/to/app   # regenerates sanitized VHS demo GIFs into assets/demos/
```
