#!/usr/bin/env bash
# The Android app's sync code (Java, on the JVM) against a real agendouille
# server, on dev-build: a server built from this tree, seeded from
# scripts/fixtures/live.seed with the shared iCalendar fixtures
# (testdata/ical), a device; then android/test/.../LiveCheck discovers,
# syncs and maps everything.
#
#   scripts/remote.sh 'bash scripts/android-live.sh'
set -euo pipefail
# shellcheck source=SCRIPTDIR/lib/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
cd "$AGD_ROOT"

admin=127.0.0.1:18894
public=127.0.0.1:18893

bin_dir=$(mktemp -d)
B="$bin_dir/agendouille"
go build -o "$B" ./cmd/agendouille
D=$(mktemp -d)
export AGENDOUILLE_DATA_DIR=$D AGENDOUILLE_ADMIN_LISTEN=$admin
export AGENDOUILLE_PUBLIC_LISTEN=$public AGENDOUILLE_PUBLIC_URL=http://$public AGENDOUILLE_PUBLIC_ALLOW_INSECURE=true
printf 'live check password\n' | "$B" admin passwd >/dev/null
agd_seed "$B" "$AGD_FIXTURES/live.seed"
for f in testdata/ical/*.ics; do "$B" import lea~perso "$f" >/dev/null; done
pass=$("$B" device add lea --label "LiveCheck" | awk '/password:/ { print $2 }')

"$B" serve 2>"$D/log" &
pid=$!
trap 'kill $pid 2>/dev/null; rm -rf "$D" "$bin_dir"' EXIT
agd_wait "http://$public/healthz"
make -s -C android live-check SERVER="http://$public" LIVE_USER=lea LIVE_PASS="$pass"
