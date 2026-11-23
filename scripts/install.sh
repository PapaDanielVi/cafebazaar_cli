#!/usr/bin/env bash
set -euo pipefail

REPO="PapaDanielVi/cafebazaar_cli"
BINARY_NAME="bazaar"
VERSION="${BAZAAR_VERSION:-}"
INSTALL_DIR="${BAZAAR_INSTALL_DIR:-}"

usage() {
  cat <<EOF
Install ${BINARY_NAME} without needing Go installed.

Usage:
  curl -fsSL https://raw.githubusercontent.com/${REPO}/master/scripts/install.sh | bash
  curl -fsSL https://raw.githubusercontent.com/${REPO}/master/scripts/install.sh | bash -s -- [-v <tag>] [-d <dir>]

Options:
  -v, --version <tag>   Install a specific release tag (e.g. v0.1.0-beta.1)
  -d, --dir <path>      Target installation directory
  -h, --help            Show this help message

Environment variables:
  BAZAAR_VERSION        Release tag to install (defaults to latest release)
  BAZAAR_INSTALL_DIR    Installation directory override
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--version)
      VERSION="${2:-}"
      shift 2
      ;;
    -d|--dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: required command '$1' not found." >&2
    exit 1
  fi
}

need_cmd curl

detect_os() {
  local uname_s
  uname_s="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$uname_s" in
    linux*) echo "linux" ;;
    darwin*) echo "darwin" ;;
    msys*|mingw*|cygwin*) echo "windows" ;;
    *)
      echo "Error: unsupported OS '$uname_s'." >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  local uname_m
  uname_m="$(uname -m | tr '[:upper:]' '[:lower:]')"
  case "$uname_m" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "Error: unsupported architecture '$uname_m'." >&2
      exit 1
      ;;
  esac
}

resolve_latest_version() {
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"
  local tag=""
  tag="$(curl -fsSL "$api_url" 2>/dev/null | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1 || true)"
  if [[ -z "$tag" ]]; then
    # Fall back to the newest release (including beta prereleases).
    api_url="https://api.github.com/repos/${REPO}/releases?per_page=1"
    tag="$(curl -fsSL "$api_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1 || true)"
  fi
  if [[ -z "$tag" ]]; then
    echo "Error: could not determine latest release for ${REPO}." >&2
    exit 1
  fi
  echo "$tag"
}

verify_checksum() {
  local file_path="$1"
  local checksums_file="$2"
  local expected_name="$3"

  local expected_hash
  expected_hash="$(awk -v name="$expected_name" '$2 == name { print $1; exit }' "$checksums_file")"
  if [[ -z "$expected_hash" ]]; then
    echo "Warning: checksum entry for ${expected_name} not found; skipping verification." >&2
    return 0
  fi

  local actual_hash=""
  if command -v sha256sum >/dev/null 2>&1; then
    actual_hash="$(sha256sum "$file_path" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual_hash="$(shasum -a 256 "$file_path" | awk '{print $1}')"
  else
    echo "Warning: neither sha256sum nor shasum found; skipping checksum verification." >&2
    return 0
  fi

  if [[ "$actual_hash" != "$expected_hash" ]]; then
    echo "Error: checksum mismatch for ${expected_name}." >&2
    echo "  Expected: ${expected_hash}" >&2
    echo "  Actual:   ${actual_hash}" >&2
    exit 1
  fi
}

OS="$(detect_os)"
ARCH="$(detect_arch)"

if [[ -z "$VERSION" ]]; then
  VERSION="$(resolve_latest_version)"
fi

VERSION_NO_V="${VERSION#v}"
EXT="tar.gz"
BIN_FILE="${BINARY_NAME}"
if [[ "$OS" == "windows" ]]; then
  EXT="zip"
  BIN_FILE="${BINARY_NAME}.exe"
fi

ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NO_V}_${OS}_${ARCH}.${EXT}"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
ARCHIVE_URL="${BASE_URL}/${ARCHIVE_NAME}"
CHECKSUMS_URL="${BASE_URL}/checksums.txt"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Installing ${BINARY_NAME} ${VERSION} (${OS}/${ARCH})..."
curl -fsSL "$ARCHIVE_URL" -o "${TMP_DIR}/${ARCHIVE_NAME}"

if curl -fsSL "$CHECKSUMS_URL" -o "${TMP_DIR}/checksums.txt" 2>/dev/null; then
  verify_checksum "${TMP_DIR}/${ARCHIVE_NAME}" "${TMP_DIR}/checksums.txt" "$ARCHIVE_NAME"
fi

if [[ "$EXT" == "zip" ]]; then
  need_cmd unzip
  unzip -q "${TMP_DIR}/${ARCHIVE_NAME}" -d "${TMP_DIR}/extract"
else
  need_cmd tar
  mkdir -p "${TMP_DIR}/extract"
  tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "${TMP_DIR}/extract"
fi

SRC_BIN="${TMP_DIR}/extract/${BIN_FILE}"
if [[ ! -f "$SRC_BIN" ]]; then
  echo "Error: expected binary '${BIN_FILE}' inside archive." >&2
  exit 1
fi

if [[ -z "$INSTALL_DIR" ]]; then
  if [[ "$OS" == "windows" ]]; then
    INSTALL_DIR="${HOME}/.local/bin"
  elif [[ -w "/usr/local/bin" ]]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

mkdir -p "$INSTALL_DIR"
chmod +x "$SRC_BIN"
mv -f "$SRC_BIN" "${INSTALL_DIR}/${BIN_FILE}"

echo "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BIN_FILE}"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    echo ""
    echo "Add ${INSTALL_DIR} to your PATH to run '${BINARY_NAME}' from anywhere:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac

"${INSTALL_DIR}/${BIN_FILE}" version
