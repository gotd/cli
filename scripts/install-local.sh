#!/usr/bin/env bash
# Build tg from this checkout and install it as <prefix>/bin/tg.
#
# Usage: scripts/install-local.sh [--prefix DIR] [--creds-from TG_BINARY] [--keep N]
#
# Each build is kept under <prefix>/opt/tg-cli/<version>/tg and <prefix>/bin/tg
# is switched to it atomically, so running commands are never interrupted and
# rolling back is `ln -sfn <older build> <prefix>/bin/tg`. The newest --keep
# builds (default 3) are kept.
#
# App credentials are embedded at build time, like release builds do. Sessions
# are bound to the credentials they were created with, so a rebuild must embed
# the same ones. They come from APP_ID/APP_HASH, or else are read from the
# build info of an existing tg binary: --creds-from, defaulting to the
# currently installed <prefix>/bin/tg. Their values are never printed.
set -euo pipefail

prefix="${HOME}/.local"
creds_from=""
keep=3

while (($#)); do
	case "$1" in
	--prefix) prefix="$2"; shift 2 ;;
	--creds-from) creds_from="$2"; shift 2 ;;
	--keep) keep="$2"; shift 2 ;;
	-h | --help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
	*) echo "install-local: unknown argument: $1" >&2; exit 2 ;;
	esac
done

die() { echo "install-local: $*" >&2; exit 1; }

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_dir="${prefix}/bin"
opt_dir="${prefix}/opt/tg-cli"

# Find a Go toolchain: $GO, go on PATH, or the newest under <prefix>/opt/go-*.
go_bin="${GO:-}"
if [[ -z "$go_bin" ]]; then
	if command -v go >/dev/null 2>&1; then
		go_bin="$(command -v go)"
	else
		go_bin="$(ls -d "${prefix}"/opt/go-*/bin/go 2>/dev/null | sort -V | tail -n1 || true)"
	fi
fi
[[ -x "$go_bin" ]] || die "no Go toolchain found (set GO=/path/to/go)"

# Resolve app credentials.
app_id="${APP_ID:-}"
app_hash="${APP_HASH:-}"
if [[ -z "$app_id" || -z "$app_hash" ]]; then
	src="${creds_from:-${bin_dir}/tg}"
	[[ -e "$src" ]] || die "no APP_ID/APP_HASH and no binary to read credentials from (${src}); use --creds-from"
	ldflags="$("$go_bin" version -m "$src" | sed -n 's/^[[:space:]]*build[[:space:]]*-ldflags=//p')"
	app_id="$(sed -n 's/.*-X main\.builtinAppID=\([^ "]*\).*/\1/p' <<<"$ldflags")"
	app_hash="$(sed -n 's/.*-X main\.builtinAppHash=\([^ "]*\).*/\1/p' <<<"$ldflags")"
	[[ -n "$app_id" && -n "$app_hash" ]] || die "${src} has no embedded app credentials"
	echo "install-local: embedding the app credentials of ${src}"
fi
[[ "$app_id" =~ ^[0-9]+$ ]] || die "APP_ID must be numeric"

cd "$repo"
commit="$(git rev-parse --short=12 HEAD)"
version="$(git describe --tags --always --dirty 2>/dev/null || echo "$commit")"
dest="${opt_dir}/${version}-${commit}"
if [[ -n "$(git status --porcelain)" ]]; then
	dest="${dest}-dirty-$(date +%Y%m%d%H%M%S)"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "install-local: building ${version} (${commit}) with $("$go_bin" env GOVERSION)"
CGO_ENABLED=0 "$go_bin" build -trimpath \
	-ldflags "-s -w -X main.builtinAppID=${app_id} -X main.builtinAppHash=${app_hash}" \
	-o "$tmp/tg" ./cmd/tg

# Smoke test: the binary runs and carries credentials.
"$tmp/tg" version -o json | grep -q '"builtin_credentials": true' ||
	die "built binary does not report embedded credentials"

mkdir -p "$dest" "$bin_dir"
install -m 0755 "$tmp/tg" "$dest/tg"
ln -sfn "$dest/tg" "$bin_dir/.tg.new"
mv -Tf "$bin_dir/.tg.new" "$bin_dir/tg"
echo "install-local: ${bin_dir}/tg -> ${dest}/tg"
"$bin_dir/tg" version

# Keep the newest $keep builds, never the one just installed.
mapfile -t builds < <(ls -1dt "${opt_dir}"/*/ 2>/dev/null | sed 's:/$::')
for old in "${builds[@]:$keep}"; do
	[[ "$old" == "$dest" ]] && continue
	rm -rf -- "$old"
	echo "install-local: removed old build ${old}"
done
