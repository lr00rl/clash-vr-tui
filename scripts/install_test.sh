#!/bin/sh
set -eu

repo_dir="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT INT TERM
mkdir -p "$test_dir/fakebin"

cat >"$test_dir/fakebin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
-s) printf '%s\n' Darwin ;;
-m) printf '%s\n' arm64 ;;
*) exit 2 ;;
esac
EOF

cat >"$test_dir/fakebin/curl" <<'EOF'
#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		shift
		out="$1"
	fi
	shift
done
case "$out" in
*checksums.txt)
	case "${INSTALL_TEST_CHECKSUM_MODE:-}" in
	missing) printf '%064d  another_asset.tar.gz\n' 0 >"$out" ;;
	mismatch) printf '%064d  clash-vr-tui_darwin_arm64.tar.gz\n' 0 >"$out" ;;
	*) exit 2 ;;
	esac
	;;
*) : >"$out" ;;
esac
EOF

chmod +x "$test_dir/fakebin/uname" "$test_dir/fakebin/curl"

assert_rejected() {
	mode="$1"
	want="$2"
	bindir="$test_dir/bin-$mode"
	output="$test_dir/$mode.out"
	if INSTALL_TEST_CHECKSUM_MODE="$mode" BINDIR="$bindir" \
		PATH="$test_dir/fakebin:$PATH" sh "$repo_dir/scripts/install.sh" >"$output" 2>&1; then
		echo "installer unexpectedly accepted $mode checksum" >&2
		exit 1
	fi
	if ! grep -F "$want" "$output" >/dev/null; then
		echo "installer did not report expected $mode error" >&2
		cat "$output" >&2
		exit 1
	fi
	if [ -e "$bindir/clash-vr-tui" ]; then
		echo "installer wrote a binary after $mode checksum failure" >&2
		exit 1
	fi
}

assert_rejected missing "checksum entry not found"
assert_rejected mismatch "FAILED"

echo "installer checksum failure tests passed"
