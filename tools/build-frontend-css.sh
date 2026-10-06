#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cache="$root/.tmp/frontend-css"
mkdir -p "$cache"

tailwind_version=4.1.11
daisyui_version=5.0.50
daisyui_sha256=dddf3a71d5d3bf8ddc63ba207e09e9aacc1e66e4396ec828b5366f064d7033cc

case "$(uname -s):$(uname -m)" in
  Linux:x86_64)
    tailwind_platform=linux-x64
    tailwind_sha256=64805b84af4292e043ea6f86d242f191c0ac75359c1a498455dfe6c642afdbab
    ;;
  Linux:aarch64|Linux:arm64)
    tailwind_platform=linux-arm64
    tailwind_sha256=0409aa4222969f47fa6f4160fe5387e79bf7269e7afe0e8b22f7532c98e1d314
    ;;
  Darwin:x86_64)
    tailwind_platform=macos-x64
    tailwind_sha256=76e27326506d10d50e65b751795f0537f9304ecb100abe835ec138c41774f38c
    ;;
  Darwin:arm64)
    tailwind_platform=macos-arm64
    tailwind_sha256=f5984b9c005c3e67841c33906c7a7c92e85e405f61e029e9bb62e880dd662e79
    ;;
  *)
    echo "Unsupported CSS build platform: $(uname -s) $(uname -m)" >&2
    exit 1
    ;;
esac

tailwind="$cache/tailwindcss-$tailwind_version-$tailwind_platform"
daisyui="$cache/daisyui-$daisyui_version.mjs"

download_verified() {
  local url="$1" path="$2" checksum="$3"
  if [[ ! -f "$path" ]]; then
    curl --fail --location --silent --show-error "$url" --output "$path"
  fi
  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$path" | cut -d ' ' -f 1)"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$path" | cut -d ' ' -f 1)"
  else
    echo "CSS builds require sha256sum or shasum -a 256" >&2
    exit 1
  fi
  [[ "$actual" == "$checksum" ]] || {
    rm -f "$path"
    echo "Checksum verification failed: $path" >&2
    exit 1
  }
}

download_verified \
  "https://github.com/tailwindlabs/tailwindcss/releases/download/v$tailwind_version/tailwindcss-$tailwind_platform" \
  "$tailwind" "$tailwind_sha256"
download_verified \
  "https://github.com/saadeghi/daisyui/releases/download/v$daisyui_version/daisyui.mjs" \
  "$daisyui" "$daisyui_sha256"
chmod +x "$tailwind"

cd "$root/internal/webapp/styles"
"$tailwind" \
  --input login.css \
  --output ../static/login.css \
  --minify
"$tailwind" \
  --input my-books.css \
  --output ../static/my-books.css \
  --minify
"$tailwind" \
  --input catalog-ops.css \
  --output ../static/catalog-ops.css \
  --minify
