#!/bin/sh
set -eu

repo="${REPO:-lr00rl/clash-vr-tui}"
version="${VERSION:-latest}"
bindir="${BINDIR:-$HOME/.local/bin}"
binary="clash-vr-tui"

need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "error: required command not found: $1" >&2
		exit 1
	fi
}

platform() {
	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	case "$os" in
	darwin) os="darwin" ;;
	linux) os="linux" ;;
	*)
		echo "error: unsupported OS for this installer: $os" >&2
		echo "Download a release manually from https://github.com/$repo/releases" >&2
		exit 1
		;;
	esac

	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*)
		echo "error: unsupported architecture for this installer: $arch" >&2
		echo "Download a release manually from https://github.com/$repo/releases" >&2
		exit 1
		;;
	esac

	printf '%s_%s' "$os" "$arch"
}

download() {
	url="$1"
	out="$2"
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$out"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$out" "$url"
	else
		echo "error: curl or wget is required" >&2
		exit 1
	fi
}

checksum() {
	file="$1"
	checksums="$2"
	asset="$3"

	line="$(grep -F "  $asset" "$checksums" | head -n 1 || true)"
	if [ -z "$line" ]; then
		echo "warning: checksum entry not found for $asset; skipping verification" >&2
		return
	fi

	printf '%s\n' "$line" >"$checksums.one"
	if command -v sha256sum >/dev/null 2>&1; then
		(cd "$(dirname "$file")" && sha256sum -c "$checksums.one")
	elif command -v shasum >/dev/null 2>&1; then
		(cd "$(dirname "$file")" && shasum -a 256 -c "$checksums.one")
	else
		echo "warning: sha256sum or shasum not found; skipping verification" >&2
	fi
}

need tar
platform_id="$(platform)"
asset="${binary}_${platform_id}.tar.gz"

if [ "$version" = "latest" ]; then
	base="https://github.com/$repo/releases/latest/download"
else
	base="https://github.com/$repo/releases/download/$version"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

archive="$tmp/$asset"
download "$base/$asset" "$archive"
download "$base/checksums.txt" "$tmp/checksums.txt"
checksum "$archive" "$tmp/checksums.txt" "$asset"

tar -xzf "$archive" -C "$tmp"
install -d "$bindir"
install -m 0755 "$tmp/$binary" "$bindir/$binary"

echo "installed $binary -> $bindir/$binary"
case ":$PATH:" in
*":$bindir:"*) ;;
*) echo "note: add $bindir to PATH to run '$binary' without a full path" ;;
esac
"$bindir/$binary" --version
