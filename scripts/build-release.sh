#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'build-release: %s\n' "$*" >&2
  exit 1
}

require_tool() {
  command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"
}

require_tool go
require_tool npm
require_tool git
require_tool tar

if command -v sha256sum >/dev/null 2>&1; then
  checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  checksum_tool='shasum -a 256'
else
  fail "required tool not found: sha256sum or shasum"
fi

repo_root=$(git rev-parse --show-toplevel) || fail "not inside a git repository"
cd "$repo_root"

if (($# > 1)); then
  fail "usage: scripts/build-release.sh <version>"
fi

if (($# == 1)); then
  version=${1#v}
else
  tag=$(git describe --tags --exact-match 2>/dev/null) || fail "no version supplied and HEAD is not exactly tagged"
  version=${tag#v}
fi

[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || fail "invalid version: $version"

commit=$(git rev-parse --short HEAD)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
feed_base=${FEED_BASE:-https://dl.tionharness.com}
feed_base=${feed_base%/}
binary=tionharness
main_package=./cmd/tionharness
release_dir="dist/release/$version"
stage_dir="$release_dir/.stage"
ldflags="-s -w -X github.com/bilal-arikan/tionharness/internal/api.BuildVersion=$version -X github.com/bilal-arikan/tionharness/internal/api.BuildCommit=$commit -X github.com/bilal-arikan/tionharness/internal/api.BuildDate=$build_date -X github.com/bilal-arikan/tionharness/internal/api.FeedBaseURL=$feed_base"

printf '==> Building frontend\n'
(
  cd frontend
  npm ci
  npm run build
)

embed_dir=internal/web/dist
[[ -d $embed_dir ]] || fail "embed target missing after frontend build: $embed_dir"
find "$embed_dir" -type f ! -name .gitkeep -print -quit | grep -q . || fail "embed target is empty after frontend build: $embed_dir"

mkdir -p "$stage_dir"
artifacts_file="$release_dir/.artifacts"
: >"$artifacts_file"

zip_helper="$stage_dir/create_zip.go"
cat >"$zip_helper" <<'EOF'
package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

func main() {
	input, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		panic(err)
	}
	output, err := os.Create(os.Args[2])
	if err != nil {
		panic(err)
	}
	archive := zip.NewWriter(output)
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		panic(err)
	}
	header.Name = filepath.Base(os.Args[1])
	header.Method = zip.Deflate
	entry, err := archive.CreateHeader(header)
	if err != nil {
		panic(err)
	}
	if _, err = io.Copy(entry, input); err != nil {
		panic(err)
	}
	if err = archive.Close(); err != nil {
		panic(err)
	}
	if err = output.Close(); err != nil {
		panic(err)
	}
}
EOF

targets=(linux/amd64 linux/arm64 windows/amd64 darwin/arm64 darwin/amd64)
for target in "${targets[@]}"; do
  os=${target%/*}
  arch=${target#*/}
  executable=$binary
  [[ $os == windows ]] && executable+=.exe
  target_dir="$stage_dir/${os}_${arch}"
  mkdir -p "$target_dir"

  printf '==> Building %s/%s\n' "$os" "$arch"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "$ldflags" -o "$target_dir/$executable" "$main_package"

  archive_base="${binary}_${version}_${os}_${arch}"
  if [[ $os == windows ]]; then
    archive="$archive_base.zip"
    go run "$zip_helper" "$target_dir/$executable" "$release_dir/$archive"
  else
    archive="$archive_base.tar.gz"
    tar -C "$target_dir" -czf "$release_dir/$archive" "$executable"
  fi
  printf '%s\t%s\t%s\n' "$os" "$arch" "$archive" >>"$artifacts_file"
done

(
  cd "$release_dir"
  : >SHA256SUMS
  while IFS=$'\t' read -r _ _ archive; do
    $checksum_tool "$archive" >>SHA256SUMS
  done <.artifacts
)

released_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{
  printf '{\n'
  printf '  "version": "%s",\n' "$version"
  printf '  "released_at": "%s",\n' "$released_at"
  printf '  "notes_url": "https://tionharness.com/releases/v%s",\n' "$version"
  printf '  "artifacts": [\n'
  first=1
  while IFS=$'\t' read -r os arch archive; do
    hash=$($checksum_tool "$release_dir/$archive" | awk '{print $1}')
    size=$(wc -c <"$release_dir/$archive" | tr -d '[:space:]')
    ((first == 1)) || printf ',\n'
    first=0
    printf '    { "os": "%s", "arch": "%s", "file": "%s", "url": "%s/v%s/%s", "sha256": "%s", "size": %s }' \
      "$os" "$arch" "$archive" "$feed_base" "$version" "$archive" "$hash" "$size"
  done <"$artifacts_file"
  printf '\n  ]\n}\n'
} >"$release_dir/latest.json"

rm -rf "$stage_dir"
rm "$artifacts_file"
printf '==> Release ready: %s\n' "$release_dir"
