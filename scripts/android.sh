#!/usr/bin/env bash
# The Android app, built and tested on dev-build (no Gradle, no SDK here).
#
#   scripts/android.sh test           # JVM tests of the pure-Java parts
#   scripts/android.sh apk            # android/build/agendouille.apk, on dev-build
#   scripts/android.sh cert           # the signing key's SHA-256 (assetlinks.json)
#   APP_HOST=agenda.example.org scripts/android.sh apk   # with App Links
#
# The tree is synced by scripts/remote.sh, and scripts/build/apk.sh builds
# there; the APK stays on dev-build (scripts/devdeploy.sh embeds it in the
# server it builds there).
set -euo pipefail
# shellcheck source=SCRIPTDIR/lib/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

exec "$AGD_ROOT/scripts/remote.sh" bash scripts/build/apk.sh "${1:-apk}" "$(agd_version)" "$(agd_version_code)" "'${APP_HOST:-}'" "${CLEARTEXT:-false}"
