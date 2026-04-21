#!/usr/bin/env sh
# adb-connect installer — downloads and installs the latest release from
# https://github.com/premex-ab/adb-connect/releases
#
# Usage:
#     curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh
#     curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh -s -- --prefix=/usr/local
#     curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh -s -- --version=0.1.0
set -eu

REPO="premex-ab/adb-connect"
PREFIX=""
VERSION=""

for arg in "$@"; do
  case "$arg" in
    --prefix=*) PREFIX="${arg#--prefix=}" ;;
    --version=*) VERSION="${arg#--version=}" ;;
    --help|-h)
      sed -n '2,/^set -eu/p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$arg" >&2
      exit 2
      ;;
  esac
done

if [ -z "$PREFIX" ]; then
  if [ -w "/usr/local/bin" ] 2>/dev/null; then
    PREFIX="/usr/local"
  else
    PREFIX="$HOME/.local"
  fi
fi
BIN_DIR="$PREFIX/bin"
mkdir -p "$BIN_DIR"

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    darwin) printf 'darwin' ;;
    linux)  printf 'linux' ;;
    *)
      printf 'unsupported OS: %s\n' "$os" >&2
      exit 1
      ;;
  esac
}

uname_arch() {
  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *)
      printf 'unsupported architecture: %s\n' "$arch" >&2
      exit 1
      ;;
  esac
}

need() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'required command missing: %s\n' "$1" >&2
    exit 1
  }
}

need curl
need tar
need mktemp
need sha256sum 2>/dev/null || need shasum

OS=$(uname_os)
ARCH=$(uname_arch)

if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name" *: *"v\{0,1\}\([^"]*\)".*/\1/p' | head -n1)
  if [ -z "$VERSION" ]; then
    printf 'failed to resolve latest version from GitHub API\n' >&2
    exit 1
  fi
fi

ASSET="adb-connect_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/v${VERSION}"
ARCHIVE_URL="$BASE_URL/$ASSET"
CHECKSUMS_URL="$BASE_URL/checksums.txt"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM

printf 'Downloading %s...\n' "$ASSET"
curl -fsSL -o "$TMP/$ASSET" "$ARCHIVE_URL"
curl -fsSL -o "$TMP/checksums.txt" "$CHECKSUMS_URL"

cd "$TMP"
printf 'Verifying checksum...\n'
# shellcheck disable=SC2015
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$ASSET" | awk '{print $1}')
else
  got=$(shasum -a 256 "$ASSET" | awk '{print $1}')
fi
want=$(awk -v a="$ASSET" '$2 == a { print $1 }' checksums.txt)
if [ -z "$want" ]; then
  printf 'no checksum entry for %s\n' "$ASSET" >&2
  exit 1
fi
if [ "$got" != "$want" ]; then
  printf 'checksum mismatch:\n  got  %s\n  want %s\n' "$got" "$want" >&2
  exit 1
fi

printf 'Extracting...\n'
tar -xzf "$ASSET"
install -m 0755 adb-connect "$BIN_DIR/adb-connect"

printf '\nInstalled adb-connect to %s\n' "$BIN_DIR/adb-connect"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) printf 'NOTE: %s is not in $PATH. Add it, e.g. for zsh:\n  echo \x27export PATH="%s:\$PATH"\x27 >> ~/.zshrc\n' "$BIN_DIR" "$BIN_DIR" ;;
esac
printf 'Next: run \x27adb-connect pair\x27 for same-LAN pairing, or \x27adb-connect remote setup\x27 for Tailscale setup.\n'
