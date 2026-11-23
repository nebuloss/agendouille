#!/usr/bin/env bash
# A visual check of the admin UI on dev-build: builds the bundles and the
# binary from the synced tree, seeds a fresh data directory, starts the
# server on 127.0.0.1:18892 (admin) and :18891 (public), and shoots every
# admin page with Chromium. Run it there:
#
#   scripts/remote.sh 'bash scripts/shots/run.sh'
#   scp -r guillaume@10.0.50.21:agendouille-shots/out /tmp/agendouille-shots
set -euo pipefail
cd "$(dirname "$0")/../.."
out="$HOME/agendouille-shots/out"
rm -rf "$out" && mkdir -p "$out"
npm run build >/dev/null
go build -o "$HOME/agendouille-shots/agendouille" ./cmd/agendouille
B="$HOME/agendouille-shots/agendouille"
D=$(mktemp -d)
export AGENDOUILLE_DATA_DIR=$D AGENDOUILLE_ADMIN_LISTEN=127.0.0.1:18892
pw="screenshots password"
printf '%s\n' "$pw" | $B admin passwd >/dev/null
$B config set public_listen 127.0.0.1:18891 public_url http://127.0.0.1:18891 public_allow_insecure true >/dev/null
$B group add famille --display-name Famille >/dev/null
$B group add enfants --display-name Enfants >/dev/null
$B user add maman --display-name Maman --group famille >/dev/null
$B user add papa --display-name Papa --group famille >/dev/null
$B user add lea --display-name Léa --group famille --group enfants >/dev/null
$B calendar add maman ecole --name École >/dev/null
$B calendar add lea perso --name "Perso de Léa" >/dev/null
$B calendar share maman~ecole g:famille readwrite >/dev/null
$B calendar share maman~ecole g:enfants read >/dev/null
$B device add lea --label "Pixel de Léa" >/dev/null
$B device add maman --kind ios --label "iPhone de Maman" >/dev/null
$B serve 2>"$D/log" &
pid=$!
trap 'kill $pid 2>/dev/null; rm -rf "$D"' EXIT
sleep 1
jar="$D/jar"
curl -s -o /dev/null -c "$jar" --data-urlencode "password=$pw" http://127.0.0.1:18892/admin/signin
curl -s -o /dev/null -b "$jar" -X POST -H 'Content-Type: application/json' -H 'Sec-Fetch-Site: same-origin' http://127.0.0.1:18892/api/admin/backups
ln -sfn "$HOME/shots/node_modules" scripts/shots/node_modules
node scripts/shots/shoot.mjs http://127.0.0.1:18892 "$pw" "$out"
ls "$out"
