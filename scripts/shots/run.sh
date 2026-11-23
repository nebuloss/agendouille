#!/usr/bin/env bash
# A visual check of the admin UI and the family web app on dev-build: builds the bundles and the
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
# Settings are configuration: the environment says them here, as a file would.
export AGENDOUILLE_PUBLIC_LISTEN=127.0.0.1:18891 AGENDOUILLE_PUBLIC_URL=http://127.0.0.1:18891 AGENDOUILLE_PUBLIC_ALLOW_INSECURE=true
$B group add famille --display-name Famille >/dev/null
$B group add enfants --display-name Enfants >/dev/null
$B user add maman --display-name Maman --group famille >/dev/null
$B user add papa --display-name Papa --group famille >/dev/null
$B user add lea --display-name Léa --group famille --group enfants >/dev/null
$B calendar add maman ecole --name École --color "#e8710a" >/dev/null
$B calendar add lea perso --name "Perso de Léa" --color "#7b1fa2" >/dev/null
$B calendar share maman~ecole g:famille readwrite >/dev/null
$B calendar share maman~ecole g:enfants read >/dev/null
$B device add lea --label "Pixel de Léa" >/dev/null
$B device add maman --kind ios --label "iPhone de Maman" >/dev/null
# A week of family events, around today.
mon=$(date -d "$(date +%F) -$(( $(date +%u) - 1 )) days" +%F)
d() { date -d "$mon +$1 days" +%Y%m%d; }
ev() { printf 'BEGIN:VEVENT\r\nUID:%s@shots\r\nDTSTAMP:20260101T000000Z\r\nSUMMARY:%s\r\n%s\r\n%s\r\n%sEND:VEVENT\r\n' "$1" "$2" "$3" "$4" "${5:-}"; }
t() { printf 'DTSTART;TZID=Europe/Paris:%sT%s00' "$(d "$1")" "$2"; }
e() { printf 'DTEND;TZID=Europe/Paris:%sT%s00' "$(d "$1")" "$2"; }
{
  printf 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//shots//EN\r\n'
  ev judo Judo "$(t 2 1400)" "$(e 2 1530)" $'RRULE:FREQ=WEEKLY\r\nLOCATION:Gymnase\r\n'
  ev reunion "Réunion parents-profs" "$(t 3 1800)" "$(e 3 1930)"
  ev piscine Piscine "$(t 5 1000)" "$(e 5 1130)"
  ev sortie "Sortie scolaire" "DTSTART;VALUE=DATE:$(d 4)" "DTEND;VALUE=DATE:$(d 5)"
  printf 'END:VCALENDAR\r\n'
} >"$D/ecole.ics"
{
  printf 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//shots//EN\r\n'
  ev dentiste Dentiste "$(t 2 1500)" "$(e 2 1545)" $'BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n'
  ev anniv "Anniversaire de Mamie" "$(t 6 1200)" "$(e 6 1600)"
  ev cine "Ciné" "$(t 0 2030)" "$(e 0 2230)"
  printf 'END:VCALENDAR\r\n'
} >"$D/perso.ics"
$B import maman~ecole "$D/ecole.ics" >/dev/null
$B import lea~perso "$D/perso.ics" >/dev/null
enrol=$($B user enrol lea | grep -o 'http://[^ ]*/enrol/[^ ]*')
$B serve 2>"$D/log" &
pid=$!
trap 'kill $pid 2>/dev/null; rm -rf "$D"' EXIT
sleep 1
jar="$D/jar"
curl -s -o /dev/null -c "$jar" --data-urlencode "password=$pw" http://127.0.0.1:18892/signin
curl -s -o /dev/null -b "$jar" -X POST -H 'Content-Type: application/json' -H 'Sec-Fetch-Site: same-origin' http://127.0.0.1:18892/api/admin/backups
ln -sfn "$HOME/shots/node_modules" scripts/shots/node_modules
node scripts/shots/shoot.mjs http://127.0.0.1:18892 "$pw" "$out"
node scripts/shots/shoot-app.mjs "$enrol" "$out"
# The certificate pass: CalDAV in "both" mode, a DAVx5 device added.
curl -s -o /dev/null -b "$jar" -X PUT -H 'Content-Type: application/json' -H 'Sec-Fetch-Site: same-origin' \
  --data '{"mode":"both"}' http://127.0.0.1:18892/api/admin/dav-auth
enrol2=$($B user enrol lea | grep -o 'http://[^ ]*/enrol/[^ ]*')
node scripts/shots/shoot-app.mjs "$enrol2" "$out" certs
# The calendar's hover states and the event editor, light and dark.
enrol3=$($B user enrol lea | grep -o 'http://[^ ]*/enrol/[^ ]*')
node scripts/shots/shoot-editor.mjs "$enrol3" "$out"
ls "$out"
