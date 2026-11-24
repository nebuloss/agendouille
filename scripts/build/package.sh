#!/usr/bin/env bash
# A deployable package, on the build host: both bundles and the Android app
# embedded in one static binary, with deploy/ beside it, in a .tgz that
# deploy/install.sh installs from. Run there, in a synced tree:
#
#   bash scripts/build/package.sh VERSION CODE APP_HOST OUT.tgz
#
# scripts/devdeploy.sh calls it.
set -euo pipefail
here=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=SCRIPTDIR/../lib/common.sh
. "$here/../lib/common.sh"
load_conf "$AGD_CONF/build.conf"
cd "$AGD_ROOT"

version=${1:?version}
code=${2:?version code}
app_host=${3:-}
out=${4:?output .tgz}

if [ ! -d node_modules ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then npm ci --silent; fi
npm run build --silent >/dev/null
test -s internal/web/assets/static/app/app.js
test -s internal/web/assets/static/admin/admin.js

apk=internal/web/assets/apk
if [ -d "$HOME/$ANDROID_DIR/sdk" ]; then
	bash "$here/apk.sh" apk "$version" "$code" "$app_host" >/dev/null
	cp android/build/agendouille.apk "$apk/agendouille.apk"
	bash "$here/apk.sh" cert "$version" "$code" >"$apk/cert.txt"
	echo "$version" >"$apk/version.txt"
	say "Android app embedded"
else
	rm -f "$apk/agendouille.apk" "$apk/cert.txt" "$apk/version.txt"
	say "no Android SDK on the build host: no app embedded"
fi

pkg=$(mktemp -d)
trap 'rm -rf "$pkg"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" GOARM="$GOARM" go build -trimpath \
	-ldflags="-s -w -X main.version=$version" -o "$pkg/agendouille" ./cmd/agendouille
cp deploy/install.sh deploy/agendouille.service deploy/agendouille.openrc \
	deploy/agendouille-container.conf deploy/agendouille.yaml.example "$pkg/"
tar -C "$pkg" -czf "$out" .
say "packaged $version for linux/$GOARCH"
