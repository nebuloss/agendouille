#!/usr/bin/env bash
# The Android app's sync code (Java, on the JVM) against a real agendouille
# server, on dev-build: a server built from this tree, a user whose
# calendars hold the shared fixtures (testdata/ical), a device; then
# android/test/.../LiveCheck discovers, syncs and maps everything.
#
#   scripts/remote.sh 'bash scripts/android-live.sh'
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

B=$(mktemp -d)/agendouille
go build -o "$B" ./cmd/agendouille
D=$(mktemp -d)
export AGENDOUILLE_DATA_DIR=$D AGENDOUILLE_ADMIN_LISTEN=127.0.0.1:18894
export AGENDOUILLE_PUBLIC_LISTEN=127.0.0.1:18893 AGENDOUILLE_PUBLIC_URL=http://127.0.0.1:18893 AGENDOUILLE_PUBLIC_ALLOW_INSECURE=true
printf 'live check password\n' | "$B" admin passwd >/dev/null
"$B" group add famille >/dev/null
"$B" user add lea --group famille >/dev/null
"$B" user add maman --group famille >/dev/null
"$B" calendar add lea perso --name "Perso de Léa" --color "#7b1fa2" >/dev/null
"$B" calendar add maman ecole --name "École" --color "#e8710a" >/dev/null
"$B" calendar share maman~ecole g:famille read >/dev/null
for f in testdata/ical/*.ics; do "$B" import lea~perso "$f" >/dev/null; done
"$B" import maman~ecole testdata/ical/allday.ics >/dev/null
pass=$("$B" device add lea --label "LiveCheck" | awk '/password:/ { print $2 }')
"$B" serve 2>"$D/log" &
pid=$!
trap 'kill $pid 2>/dev/null; rm -rf "$D" "$(dirname "$B")"' EXIT
for _ in $(seq 50); do curl -fsS http://127.0.0.1:18893/healthz >/dev/null 2>&1 && break; sleep 0.1; done
make -s -C android live-check SERVER=http://127.0.0.1:18893 LIVE_USER=lea LIVE_PASS="$pass"
