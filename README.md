<div align="center">

<img src="assets/logo.svg" alt="bazaar CLI logo" width="140" height="140" />

# bazaar

**The command-line tool for CafeBazaar Android app publishing, releases, and user reviews.**

[![CI](https://github.com/PapaDanielVi/cafebazaar_cli/actions/workflows/ci.yml/badge.svg)](https://github.com/PapaDanielVi/cafebazaar_cli/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/PapaDanielVi/cafebazaar_cli?color=00B14F)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-00B14F.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-slate)](https://github.com/PapaDanielVi/cafebazaar_cli)

</div>

`bazaar` brings CafeBazaar's developer APIs into your terminal and CI/CD pipelines, modeled after `gh` for GitHub. It covers the full Pishkhan Release API, the Sandoghche Comments (Reviews) API, and the Nasher Publisher API, paired with Android Gradle auto-detection, Persian (Farsi) UTF-8 terminal rendering, and Git release automation.

---

## Features

- **Complete CafeBazaar API coverage**: all 10 Pishkhan Release API endpoints (create, delete, APK/AAB/bin upload, bundle status polling, commit, published/committed/uncommitted queries), both Comments API endpoints (filtered review listing with cursor pagination and developer replies), and the Nasher Publisher API (`POST /v1/apps/` with `issue_api_agent`).
- **Two-token architecture with automatic folder binding**: stores your developer-level Nasher token and every per-app Pishkhan token inside your OS keychain (with `0600` file fallback, never inside your Git repo). Once initialized, `cd` into any Android app folder and `bazaar` automatically selects that app's secret key.
- **Android Gradle awareness**: pure-Go parser for `build.gradle`, `build.gradle.kts`, `gradle.properties`, and `gradle/libs.versions.toml` that detects all `buildTypes` and `productFlavors`, resolves every variant's package name, discovers APK/AAB/`.bin` artifacts, and bumps `versionCode`/`versionName` in place.
- **Dynamic shell autocomplete**: tab-completes `--app`, `--variant`, and artifact file paths using your live Gradle build types, registered apps, and `build/outputs/` files.
- **Persian (Farsi) UTF-8 rendering**: pure-Go Persian glyph shaper (all 32 Persian letters, Lam-Alef ligatures, ZWNJ `\u200c`, embedded LTR runs) with `--rtl auto|on|off` for terminals without native BiDi, while keeping JSON/YAML and piped stdout in unmodified logical UTF-8.
- **Interactive UI and Headless CI/CD mode**: styled emerald cards, tables, star ratings, spinners, and upload progress bars locally; automatic or flag-driven headless plain/JSON/YAML mode (`--no-ui`, `--plain`, `CI=true`) with full environment variable support and deterministic exit codes.
- **Proxy support**: route CafeBazaar API calls and uploads through HTTP, HTTPS, or SOCKS5 (`socks5://` / `socks5h://`) proxies via `--proxy`, `BAZAAR_PROXY` / `CAFEBAZAAR_PROXY`, `.bazaar.yaml`, `~/.config/bazaar/config.yaml`, or standard `HTTP_PROXY`/`HTTPS_PROXY` variables.
- **Safety confirmations**: every destructive or externally visible action (`release delete`, `release commit`, `publish`, `review reply`, `app create`, token overwrite/removal) asks for confirmation interactively (defaulting to No) and refuses to run in CI without `--yes` (`-y`) or `CAFEBAZAAR_YES=1`.

---

## Installation

No Go toolchain is required for prebuilt binaries. Pick the one-liner for your platform below, or install via Go.

### Linux, macOS, and Windows (Git Bash / WSL)

```bash
curl -fsSL https://raw.githubusercontent.com/PapaDanielVi/cafebazaar_cli/master/scripts/install.sh | bash
```

Install a specific version or custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/PapaDanielVi/cafebazaar_cli/master/scripts/install.sh | bash -s -- -v v0.1.0-beta.3 -d "$HOME/.local/bin"
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/PapaDanielVi/cafebazaar_cli/master/scripts/install.ps1 | iex
```

To pin a version in PowerShell:

```powershell
$env:BAZAAR_VERSION = "v0.1.0-beta.3"
irm https://raw.githubusercontent.com/PapaDanielVi/cafebazaar_cli/master/scripts/install.ps1 | iex
```

### Using Go

```bash
go install github.com/PapaDanielVi/cafebazaar_cli/cmd/bazaar@latest
```

### From Source

```bash
git clone https://github.com/PapaDanielVi/cafebazaar_cli.git
cd cafebazaar_cli
make build
# Binary is built at ./bin/bazaar

# Or install into $GOBIN:
make install
```

### Shell Autocomplete

Enabling completions unlocks dynamic tab suggestions for your Gradle build-type package names, variants, and build artifacts:

```bash
# Zsh
bazaar completion zsh > "${fpath[1]}/_bazaar"

# Bash
source <(bazaar completion bash)

# Fish
bazaar completion fish | source

# PowerShell
bazaar completion powershell | Out-String | Invoke-Expression
```

---

## Quickstart

### 1. Log in with your Nasher (Publisher) developer token

Generate a developer Nasher token at `https://pishkhan.cafebazaar.ir/settings/publisher-api` and save it once:

```bash
bazaar auth login
```

### 2. Initialize your Android app folder

Navigate to your Android project and run `bazaar init`:

```bash
cd /path/to/your-android-app
bazaar init
```

`bazaar init` scans your Gradle configuration, lists every detected `buildType` and `productFlavor` package name, and walks you through one of two paths:

- **Existing app on CafeBazaar**: open your app in Pishkhan (`App -> API پیشخان بازار -> دریافت توکن جدید`), paste the generated app token, and `bazaar` verifies it live against Pishkhan, saves it in your OS keychain (or `0600` credentials file), binds the folder, and writes a non-secret `.bazaar.yaml`.
- **New app**: uses your saved Nasher developer token to submit the initial app (`POST /v1/apps/` with `issue_api_agent=true`), captures the returned app token automatically, and saves it in the background store.

From that point on, any `bazaar` command you run inside that folder or its subdirectories uses that app's stored secret automatically.

### 3. Inspect status or ship a release

```bash
# Combined local Gradle version, Git state, and live CafeBazaar releases
bazaar status

# Preview an end-to-end release without mutating Gradle, Git, or CafeBazaar
bazaar publish --bump patch --build --git-commit --git-tag --dry-run

# Run the full Android publish flow
bazaar publish --bump patch --build --git-commit --git-tag
```

---

## Command Reference

### Authentication (`bazaar auth`)

```bash
bazaar auth login                     # Save developer Nasher token (alias for auth publisher login)
bazaar auth status                    # Show developer token, registered apps, and active folder app
bazaar auth publisher status          # Check developer Nasher token status
bazaar auth publisher logout          # Remove stored developer Nasher token (asks confirmation)
bazaar auth app add [package-name]    # Add per-app Pishkhan token (suggests Gradle build types)
bazaar auth app list                  # List registered apps and bound project folders
bazaar auth app status                # Validate active app token against CafeBazaar
bazaar auth app remove <package-name> # Remove stored app token (asks confirmation)
```

### App Management (`bazaar init`, `bazaar app`, `bazaar status`)

```bash
bazaar init                           # Guided setup for existing or new Android apps
bazaar status                         # Unified local Gradle + Git + CafeBazaar release dashboard
bazaar app list                       # List all configured apps
bazaar app use <package-name>         # Set global default app (--local binds current directory)
bazaar app create --package com.x.app --artifact app-release.apk
```

### Android Gradle Tools (`bazaar android`)

```bash
bazaar android info                   # Show detected module, variants, versions, artifacts, and Git info
bazaar android variants               # Table of all Gradle buildTypes/flavors and resolved package names
bazaar android bump --bump patch --sync-remote --git-commit --git-tag
```

`--sync-remote` queries CafeBazaar's live published and committed releases first so `versionCode` is guaranteed to be higher than both your local Gradle file and CafeBazaar.

### Pishkhan Release API (`bazaar release`)

Direct 1-to-1 commands for all 10 Pishkhan endpoints on `https://api.pishkhan.cafebazaar.ir`:

| Command | HTTP Method & Endpoint | Notes |
|---|---|---|
| `bazaar release create` | `POST /v1/apps/releases/` | Creates an uncommitted draft release |
| `bazaar release delete` | `DELETE /v1/apps/releases/` | Deletes uncommitted draft (requires confirmation) |
| `bazaar release upload-apk [file]` | `POST /v1/apps/releases/upload/` | Supports `--arch all\|armeabi-v7a\|arm64-v8a` |
| `bazaar release upload-aab [file]` | `POST /v1/apps/releases/upload-aab/` | Uploads Android App Bundle (`state=U`) |
| `bazaar release upload-bin [file]` | `POST /v1/apps/releases/upload-bin/` | Uploads Bundle Signer `.bin` (`state=S`) |
| `bazaar release bundle-status` | `GET /v1/apps/releases/bundle-status/` | Pass `--watch` to poll every 60s until `D` or `E` |
| `bazaar release commit` | `POST /v1/apps/releases/commit/` | Submits for review (requires confirmation) |
| `bazaar release status --only published` | `GET /v1/apps/releases/last-published/` | Live published release |
| `bazaar release status --only committed` | `GET /v1/apps/releases/last-committed/` | Most recent committed release |
| `bazaar release status --only uncommitted` | `GET /v1/apps/releases/last-uncommitted/` | Current draft release |

Example APK release step by step:

```bash
bazaar release create
bazaar release upload-apk app/build/outputs/apk/release/app-release.apk --arch all
bazaar release commit \
  --changelog-fa-file changelogs/fa.txt \
  --changelog-en "Bug fixes and speed improvements" \
  --note "Tested on Android 14" \
  --rollout 25 \
  --auto-publish
```

Example AAB release step by step:

```bash
bazaar release create
bazaar release upload-aab app/build/outputs/bundle/release/app-release.aab
bazaar release upload-bin app/build/outputs/bundle/release/app-release.bin
bazaar release bundle-status --watch
bazaar release commit --changelog-fa "بهبود عملکرد و رفع خطاها"
```

### End-to-End Pipeline (`bazaar publish`)

Orchestrates detection, remote status check, version bump, optional Gradle build, optional Git commit/tag/push, release creation, APK or AAB+`.bin` upload, bundle polling, and commit in a single command:

```bash
bazaar publish \
  --variant release \
  --bump minor \
  --build \
  --format aab \
  --git-commit \
  --git-tag \
  --changelog-fa-file changelogs/fa.txt \
  --rollout 50
```

### User Reviews (`bazaar review`)

Full access to CafeBazaar's Sandoghche Comments API (`https://sandoghche.cafebazaar.ir`):

```bash
# Filter unreplied 1 to 3 star reviews for versionCode 42
bazaar review list --unreplied --rate-min 1 --rate-max 3 --version-code 42 --all --limit 50

# Filter by date range and Android SDK version, output raw UTF-8 JSON
bazaar review list --after 2025-01-01T00:00:00 --sdk-version 34 --output json

# Reply to a review (requires confirmation)
bazaar review reply 122455833 --text "ممنون از بازخوردتون، در نسخه جدید مشکل برطرف شد."
```

---

## Persian (Farsi) UTF-8 Handling

Most CafeBazaar changelogs, user comments, and API error messages are written in Persian. `bazaar` preserves UTF-8 end to end:

- Outgoing HTTP JSON uses `Content-Type: application/json;charset=UTF-8` with HTML escaping disabled.
- `--changelog-fa-file`, `--changelog-en-file`, and `--text-file` read UTF-8 files cleanly without shell quoting issues.
- `--rtl auto` (default) checks whether your terminal supports native BiDi/shaping (WezTerm, Ghostty, Konsole, VTE, Windows Terminal, Apple Terminal). On terminals without native RTL shaping (such as Alacritty, tmux, or unconfigured iTerm2), `bazaar` automatically reshapes Persian glyphs (including `پ چ ژ گ ک ی`, Lam-Alef ligatures, and ZWNJ `\u200c`) and preserves embedded LTR tokens like version numbers and package names.
- Override anytime with `--rtl on`, `--rtl off`, or `BAZAAR_RTL=off`.
- `--output json`, `--output yaml`, and piped non-TTY output always emit raw logical UTF-8.

---

## Proxy Support

`bazaar` supports HTTP, HTTPS, and SOCKS5 proxies (with optional `user:password` credentials) for all CafeBazaar API requests and APK/AAB uploads. This is helpful when publishing from CI runners outside Iran or behind corporate firewalls.

Supported proxy formats:
- `http://proxy.example.com:8080`
- `http://user:pass@proxy.example.com:8080`
- `https://proxy.example.com:8443`
- `socks5://127.0.0.1:1080` or `socks5h://user:pass@127.0.0.1:1080`
- `direct` or `none` (ignores system `HTTP_PROXY`/`HTTPS_PROXY` variables and connects directly)

Precedence order (highest first):
1. `--proxy <url>` CLI flag
2. `BAZAAR_PROXY` or `CAFEBAZAAR_PROXY` environment variable (scopes proxy to `bazaar` only, leaving Gradle and Git untouched)
3. `proxy` in `.bazaar.yaml`
4. `proxy` in `~/.config/bazaar/config.yaml`
5. Standard `HTTPS_PROXY`, `HTTP_PROXY`, `ALL_PROXY`, and `NO_PROXY` environment variables

Examples:

```bash
# Direct CLI flag
bazaar publish --proxy socks5://127.0.0.1:1080 --apk app-release.apk

# Environment variable in CI (does not affect ./gradlew)
export CAFEBAZAAR_PROXY="http://user:pass@ir-proxy.example.com:8080"
bazaar publish --apk app-release.apk --yes --no-ui

# Bypass a system VPN/proxy for CafeBazaar only
bazaar status --proxy direct
```

---

## Headless CI/CD Mode

In CI pipelines (`CI=true`, non-TTY streams, `--no-ui`, or `--plain`), `bazaar` switches to headless mode with zero TUI redraws, no ANSI colors, milestone upload logs (`25%`, `50%`, `75%`, `100%`), and predictable exit codes.

### GitHub Actions Example

```yaml
name: Publish to CafeBazaar

on:
  push:
    tags:
      - "v*"

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build release APK
        run: ./gradlew :app:assembleRelease

      - name: Install bazaar CLI
        run: curl -fsSL https://raw.githubusercontent.com/PapaDanielVi/cafebazaar_cli/master/scripts/install.sh | bash

      - name: Upload and submit to CafeBazaar
        env:
          CAFEBAZAAR_APP_TOKEN: ${{ secrets.CAFEBAZAAR_APP_TOKEN }}
          CAFEBAZAAR_YES: "1"
          BAZAAR_NO_UI: "1"
        run: |
          bazaar publish \
            --apk app/build/outputs/apk/release/app-release.apk \
            --arch all \
            --changelog-fa-file changelogs/fa.txt \
            --changelog-en "Release ${{ github.ref_name }}" \
            --rollout 100 \
            --output json
```

### GitLab CI Example

Add `CAFEBAZAAR_APP_TOKEN` as a masked CI/CD variable under **Settings -> CI/CD -> Variables**, then add this to `.gitlab-ci.yml`:

```yaml
stages:
  - build
  - publish

build_apk:
  stage: build
  image: ghcr.io/cirruslabs/android-sdk:35
  rules:
    - if: '$CI_COMMIT_TAG =~ /^v/'
  script:
    - ./gradlew :app:assembleRelease
  artifacts:
    paths:
      - app/build/outputs/apk/release/*.apk
    expire_in: 1 day

publish_cafebazaar:
  stage: publish
  image: debian:bookworm-slim
  needs:
    - build_apk
  rules:
    - if: '$CI_COMMIT_TAG =~ /^v/'
  variables:
    CAFEBAZAAR_YES: "1"
    BAZAAR_NO_UI: "1"
  before_script:
    - apt-get update && apt-get install -y --no-install-recommends curl ca-certificates tar
    - curl -fsSL https://raw.githubusercontent.com/PapaDanielVi/cafebazaar_cli/master/scripts/install.sh | bash
  script:
    - |
      bazaar publish \
        --apk app/build/outputs/apk/release/app-release.apk \
        --arch all \
        --changelog-fa-file changelogs/fa.txt \
        --changelog-en "Release ${CI_COMMIT_TAG}" \
        --rollout 100 \
        --output json
```

### Environment Variables

| Variable | Description |
|---|---|
| `CAFEBAZAAR_PUBLISHER_TOKEN` | Developer-account Nasher token |
| `CAFEBAZAAR_APP_TOKEN` | Per-app Pishkhan token (bypasses local token store) |
| `CAFEBAZAAR_APP` | Target registered package name or alias |
| `CAFEBAZAAR_VARIANT` | Gradle build variant |
| `CAFEBAZAAR_ARCH` | APK CPU architecture (`all`, `armeabi-v7a`, `arm64-v8a`) |
| `CAFEBAZAAR_AUTO_PUBLISH` | `true` or `false` for commit/publish |
| `CAFEBAZAAR_ROLLOUT` | Staged rollout percentage (1 to 100) |
| `CAFEBAZAAR_CHANGELOG_FA` | Persian release notes |
| `CAFEBAZAAR_CHANGELOG_EN` | English release notes |
| `CAFEBAZAAR_DEV_NOTE` | Developer note for CafeBazaar review team |
| `CAFEBAZAAR_YES` | Bypass confirmation prompts in CI (`1` or `true`) |
| `BAZAAR_NO_UI` / `BAZAAR_PLAIN` / `CI` | Enable headless plain CLI mode |
| `BAZAAR_OUTPUT` | Output format (`table`, `plain`, `json`, `yaml`) |
| `BAZAAR_RTL` | Persian terminal shaping (`auto`, `on`, `off`) |
| `BAZAAR_NO_KEYRING` | Skip OS keychain and use `0600` credentials file |
| `BAZAAR_CONFIG` | Custom global `config.yaml` path |
| `BAZAAR_PROXY` / `CAFEBAZAAR_PROXY` | HTTP, HTTPS, or SOCKS5 proxy URL (or `direct` to bypass system proxy) |
| `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` | Standard system proxy fallback when no explicit bazaar proxy is set |
| `NO_COLOR` | Disable ANSI color output |

### Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | General runtime error |
| `2` | Invalid flags or arguments |
| `3` | Authentication error or uninitialized app |
| `4` | Dangerous action cancelled or missing `--yes` in CI |
| `5` | CafeBazaar API error or AAB bundle processing failure |

---

## Project Configuration (`.bazaar.yaml`)

Running `bazaar init` writes a non-secret `.bazaar.yaml` in your Android project root. Secrets are never written to this file, so it is safe to check into Git if you want shared defaults across your team:

```yaml
app: com.example.myapp
module: :app
default_variant: release
apps:
  release: com.example.myapp
  staging: com.example.myapp.staging
  debug: com.example.myapp.debug
proxy: "http://proxy.example.com:8080"
tag_prefix: v
commit_template: "chore(release): bump version to v%s (%d)"
auto_publish: false
staged_rollout: 100
bundle_signer_cmd: "java -jar tools/bundle-signer.jar {aab}"
```

---

## Security and Credential Storage

- Tokens are stored in your native OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager) under service `bazaar-cli`.
- On headless machines where no keychain daemon is available (or when `BAZAAR_NO_KEYRING=1`), tokens fall back to `~/.config/bazaar/credentials.yaml` created atomically with `0600` permissions inside a `0700` directory.
- App folder bindings live in `~/.config/bazaar/config.yaml`.

---

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, coding standards, and pull request guidelines.

## License

Released under the [MIT License](LICENSE).
