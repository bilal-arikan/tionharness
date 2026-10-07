#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  printf 'Kullanım: %s <version>\n' "$0" >&2
  exit 2
fi

version=$1
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
source_dir="$repo_root/dist/release/$version"
source_latest="$source_dir/latest.json"
destination_dir="$script_dir/srv/dl/v$version"

if [ ! -d "$source_dir" ]; then
  printf 'Hata: kaynak sürüm dizini bulunamadı: %s\n' "$source_dir" >&2
  exit 1
fi

if [ ! -f "$source_latest" ]; then
  printf 'Hata: latest.json bulunamadı: %s\n' "$source_latest" >&2
  exit 1
fi

mkdir -p "$destination_dir"
cp -R "$source_dir"/. "$destination_dir"/
cp "$source_latest" "$script_dir/srv/dl/latest.json"
