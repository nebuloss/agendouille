#!/usr/bin/env bash
# A visual check of the admin UI and the family web app on dev-build: builds
# the bundles and the binary from the synced tree, seeds a fresh data
# directory (scripts/fixtures: the family, and this week's events from the
# .ics.in templates), starts the server on 127.0.0.1:18892 (admin) and
# :18891 (public), and shoots the pages with Chromium. Run it there:
#
#   scripts/remote.sh 'bash scripts/shots/run.sh'
#   scp -r guillaume@10.0.50.21:agendouille-shots/out /tmp/agendouille-shots
set -euo pipefail
# shellcheck source=SCRIPTDIR/../lib/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"
cd "$AGD_ROOT"

admin=127.0.0.1:18892
public=127.0.0.1:18891
pw="screenshots password"
work="$HOME/agendouille-shots"
out="$work/out"
rm -rf "$out" && mkdir -p "$out"

npm run build >/dev/null
B="$work/agendouille"
go build -o "$B" ./cmd/agendouille

D=$(mktemp -d)
# Settings are configuration: the environment says them here, as a file would.
export AGENDOUILLE_DATA_DIR=$D AGENDOUILLE_ADMIN_LISTEN=$admin
export AGENDOUILLE_PUBLIC_LISTEN=$public AGENDOUILLE_PUBLIC_URL=http://$public AGENDOUILLE_PUBLIC_ALLOW_INSECURE=true
printf '%s\n' "$pw" | "$B" admin passwd >/dev/null

# This week's events, around today, then the family (its seed imports them).
monday=$(date -d "$(date +%F) -$(($(date +%u) - 1)) days" +%F)
agd_week "$AGD_FIXTURES/ecole.ics.in" "$D/ecole.ics" "$monday"
agd_week "$AGD_FIXTURES/perso.ics.in" "$D/perso.ics" "$monday"
agd_seed "$B" "$AGD_FIXTURES/family.seed"

# A new sign-in link for Léa, for each pass of the family web app.
enroll() { "$B" user enroll lea | grep -o 'http://[^ ]*/enroll/[^ ]*'; }
first=$(enroll)

"$B" serve 2>"$D/log" &
pid=$!
trap 'kill $pid 2>/dev/null; rm -rf "$D"' EXIT
agd_wait "http://$admin/healthz"

# The admin's session, and a backup for the Backups page.
jar="$D/jar"
admin_api() { curl -s -o /dev/null -b "$jar" -H 'Content-Type: application/json' -H 'Sec-Fetch-Site: same-origin' "$@"; }
curl -s -o /dev/null -c "$jar" --data-urlencode "password=$pw" "http://$admin/signin"
admin_api -X POST "http://$admin/api/admin/backups"

ln -sfn "$HOME/shots/node_modules" scripts/shots/node_modules
node scripts/shots/shoot.mjs "http://$admin" "$pw" "$out"
node scripts/shots/shoot-app.mjs "$first" "$out"
# The certificate pass: CalDAV in "both" mode, a DAVx5 device added.
admin_api -X PUT --data '{"mode":"both"}' "http://$admin/api/admin/dav-auth"
node scripts/shots/shoot-app.mjs "$(enroll)" "$out" certs
# The calendar's hover states and the event editor, light and dark.
node scripts/shots/shoot-editor.mjs "$(enroll)" "$out"
# The English pass, both UIs.
node scripts/shots/shoot-en.mjs "$(enroll)" "$out" "$jar"
ls "$out"
