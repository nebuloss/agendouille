#!/usr/bin/env bash
# The Android app, on the build host (its SDK, android/Makefile; no Gradle).
# Signed with the release key when the host has it (scripts/conf/build.conf),
# else the debug key. Run there, in a synced tree:
#
#   bash scripts/build/apk.sh apk|test|cert VERSION CODE [APP_HOST] [CLEARTEXT]
#
# scripts/android.sh and scripts/build/package.sh call it.
set -euo pipefail
# shellcheck source=SCRIPTDIR/../lib/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"
load_conf "$AGD_CONF/build.conf"
cd "$AGD_ROOT"

target=${1:?apk, test or cert}
version=${2:?version}
code=${3:?version code}
app_host=${4:-}
cleartext=${5:-false}

# rtk trims make's noise when it is there.
run() { if command -v rtk >/dev/null; then rtk "$@"; else "$@"; fi; }

signing=()
if [ -f "$HOME/$RELEASE_KEYSTORE" ]; then
	signing=(KEYSTORE="$HOME/$RELEASE_KEYSTORE" KS_PASS="$(cat "$HOME/$RELEASE_KEYSTORE_PASSWORD_FILE")" KEY_ALIAS="$RELEASE_KEY_ALIAS")
fi

# The screens (src/android) go into the APK: built first.
[ "$target" != apk ] || npm run build --silent >/dev/null
case $target in
cert) make -s -C android cert "${signing[@]}" ;;
*) run make -C android "$target" VERSION="$version" VERSION_CODE="$code" APP_HOST="$app_host" CLEARTEXT="$cleartext" "${signing[@]}" ;;
esac
