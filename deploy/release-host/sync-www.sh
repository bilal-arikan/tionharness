#!/usr/bin/env bash
set -euo pipefail

# Publishes the promo site (website/dist, produced by `npm run build` in
# website/) to the www root this Caddy unit serves.
#
# Two modes, picked by whether RELEASE_HOST is set:
#   local  (default)      -> copy into ./srv/www, which docker-compose mounts
#                            read-only at /srv/www for the {$WWW_SITE} block.
#   remote (RELEASE_HOST) -> rsync over ssh, mirroring the transfer in
#                            .gitea/workflows/release.yml. RELEASE_WWW_PATH is
#                            the www root there; it must NOT be RELEASE_PATH,
#                            which is the download root and would be wiped by
#                            the --delete below.

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
source_dir="$repo_root/website/dist"

if [ ! -d "$source_dir" ]; then
  printf 'Hata: site çıktısı bulunamadı: %s\n' "$source_dir" >&2
  printf 'Önce `cd website && npm run build` çalıştırın.\n' >&2
  exit 1
fi

if [ ! -f "$source_dir/index.html" ]; then
  printf 'Hata: %s/index.html yok — çıktı eksik görünüyor.\n' "$source_dir" >&2
  exit 1
fi

if [ -z "${RELEASE_HOST:-}" ]; then
  destination_dir="$script_dir/srv/www"
  mkdir -p "$destination_dir"
  # Stale files from a previous build would keep being served, so clear first.
  find "$destination_dir" -mindepth 1 ! -name .gitkeep -exec rm -rf -- {} +
  cp -R "$source_dir"/. "$destination_dir"/
  printf 'Yayımlandı (yerel): %s -> %s\n' "$source_dir" "$destination_dir"
  exit 0
fi

for name in RELEASE_USER RELEASE_WWW_PATH; do
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    printf 'Hata: RELEASE_HOST verildi ama %s boş — uzak yayım iptal.\n' "$name" >&2
    exit 1
  fi
done

port="${RELEASE_PORT:-22}"
dest_root="${RELEASE_WWW_PATH%/}"
ssh_cmd="ssh -p $port"
if [ -n "${RELEASE_SSH_KEY:-}" ]; then
  ssh_cmd="$ssh_cmd -i $RELEASE_SSH_KEY"
fi

$ssh_cmd "$RELEASE_USER@$RELEASE_HOST" "mkdir -p '$dest_root'"
rsync -az --delete -e "$ssh_cmd" \
  "$source_dir/" "$RELEASE_USER@$RELEASE_HOST:$dest_root/"
printf 'Yayımlandı (uzak): %s -> %s@%s:%s\n' \
  "$source_dir" "$RELEASE_USER" "$RELEASE_HOST" "$dest_root"
