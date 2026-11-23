# bazaar

A Go CLI for managing CafeBazaar Android apps, releases, and user reviews from your terminal or CI/CD pipelines.

## Install

```bash
make build
# Binary is created at bin/bazaar

# Or install into $GOBIN:
make install
```

Enable shell autocompletion (including dynamic Android Gradle build-type package suggestions):

```bash
# Zsh
bazaar completion zsh > "${fpath[1]}/_bazaar"

# Bash
source <(bazaar completion bash)

# Fish
bazaar completion fish | source
```

## Quickstart

1. Log in with your CafeBazaar Nasher (Publisher) developer account token:

```bash
bazaar auth login
```

2. Navigate to your Android app folder and initialize it:

```bash
cd /path/to/my-android-app
bazaar init
```

`bazaar init` scans your Gradle files (`build.gradle` / `build.gradle.kts`), suggests every detected `buildTypes` and `productFlavors` package name, and lets you choose:

- **Existing app**: generate the app token in CafeBazaar Pishkhan (`App -> API Pishkhan -> Generate New Token`), paste it in, and `bazaar` validates and saves it in your OS keychain (with `0600` file fallback) while binding the folder.
- **New app**: submits the initial app via your stored Nasher developer token (`POST /v1/apps/` with `issue_api_agent=true`) and automatically saves the returned app token.

After initialization, running any `bazaar` command inside that app folder automatically loads its stored secret key.

3. Check combined local Gradle + live CafeBazaar status or ship a release:

```bash
bazaar status
bazaar publish --bump patch --build --git-commit --git-tag
```

## Commands

- `bazaar auth`: `login`, `status`, `publisher login|status|logout`, `app add|list|status|remove`
- `bazaar init`: initialize an Android folder (existing dashboard token or new Nasher submission)
- `bazaar status`: combined local Gradle version, Git branch/tag, and CafeBazaar published/committed/uncommitted releases
- `bazaar android`: `info`, `variants`, `bump` (`--bump code|patch|minor|major --sync-remote --git-commit --git-tag --dry-run`)
- `bazaar release`: 1-to-1 access to all 10 Pishkhan Release API endpoints:
  - `status [--only published|committed|uncommitted]`
  - `create` (`POST /v1/apps/releases/`)
  - `delete` (`DELETE /v1/apps/releases/`)
  - `upload-apk [file] --arch all|armeabi-v7a|arm64-v8a`
  - `upload-aab [file]`
  - `upload-bin [file]`
  - `bundle-status [--watch] [--interval 60s]`
  - `commit --changelog-fa ... --changelog-en ... --note ... --rollout 100 --auto-publish`
- `bazaar publish`: end-to-end Android publish pipeline (`--dry-run` supported)
- `bazaar review`: Sandoghche Comments API:
  - `list [--after ...] [--before ...] [--rate-min 1] [--rate-max 5] [--version-code N] [--sdk-version N] [--replied|--unreplied] [--all --limit N]`
  - `reply <review-id> --text "..." [--text-file reply.txt] [--email dev@example.com]`
- `bazaar app`: `list`, `use <pkg> [--local]`, `create`

## UTF-8 and Persian (Farsi) Support

CafeBazaar changelogs, reviews, and API errors are mostly in Persian. `bazaar` handles UTF-8 natively across all HTTP requests, files (`--changelog-fa-file`, `--text-file`), and terminal output:

- `--rtl auto` (default): detects terminals with native BiDi/shaping (WezTerm, Ghostty, Konsole, VTE, Windows Terminal, Apple Terminal) and outputs logical UTF-8 there; enables pure-Go Persian glyph shaping and BiDi ordering on terminals without RTL support.
- `--rtl on` / `--rtl off` (or `BAZAAR_RTL`): force shaping on or off.
- `--output json` and `--output yaml` always emit unmodified raw logical UTF-8 without HTML escaping.

## Headless CI/CD Mode and Environment Variables

Pass `--no-ui` or `--plain` (or run in a `CI=true` / non-TTY shell) to disable TUI styling, spinners, and prompts. Dangerous actions (`release delete`, `release commit`, `publish`, `review reply`, `app create`, `auth remove`) refuse to run in CI unless `--yes` (`-y`) or `CAFEBAZAAR_YES=1` is explicitly set.

```bash
export CAFEBAZAAR_APP_TOKEN="your-app-pishkhan-token"
export CAFEBAZAAR_YES=1
export BAZAAR_NO_UI=1

bazaar publish \
  --apk app/build/outputs/apk/release/app-release.apk \
  --arch all \
  --changelog-fa-file release-notes-fa.txt \
  --rollout 100 \
  --output json
```

Supported environment variables:

| Variable | Purpose |
|---|---|
| `CAFEBAZAAR_PUBLISHER_TOKEN` | Developer Nasher token |
| `CAFEBAZAAR_APP_TOKEN` | Per-app Pishkhan token (bypasses local token lookup) |
| `CAFEBAZAAR_APP` | Target registered package name or alias |
| `CAFEBAZAAR_VARIANT` | Gradle build variant |
| `CAFEBAZAAR_ARCH` | APK architecture (`all`, `armeabi-v7a`, `arm64-v8a`) |
| `CAFEBAZAAR_AUTO_PUBLISH` | `true` or `false` |
| `CAFEBAZAAR_ROLLOUT` | Staged rollout percentage (1 to 100) |
| `CAFEBAZAAR_CHANGELOG_FA` | Persian release notes |
| `CAFEBAZAAR_CHANGELOG_EN` | English release notes |
| `CAFEBAZAAR_DEV_NOTE` | Note for CafeBazaar review team |
| `CAFEBAZAAR_YES` | Bypass confirmation prompts in CI (`1`/`true`) |
| `BAZAAR_NO_UI` / `BAZAAR_PLAIN` / `CI` | Enable headless plain CLI mode |
| `BAZAAR_OUTPUT` | Output format (`table`, `plain`, `json`, `yaml`) |
| `BAZAAR_RTL` | Persian terminal shaping (`auto`, `on`, `off`) |
| `BAZAAR_NO_KEYRING` | Skip OS keychain and use `0600` credentials file |
| `BAZAAR_CONFIG` | Custom global `config.yaml` path |
| `NO_COLOR` | Disable ANSI colors |
