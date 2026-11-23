#!/usr/bin/env bash
# The Android app, built and tested on dev-build (no Gradle, no SDK here).
#
#   scripts/android.sh test           # JVM tests of the pure-Java parts
#   scripts/android.sh apk            # android/build/agendouille.apk, on dev-build
#   scripts/android.sh cert           # the signing key's SHA-256 (assetlinks.json)
#   APP_HOST=agenda.example.org scripts/android.sh apk   # with App Links
#
# The tree is synced by scripts/remote.sh; the APK stays on dev-build
# (scripts/devdeploy.sh embeds it in the server it builds there).
set -euo pipefail
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

target=${1:-apk}
version="dev-$(git rev-parse --short HEAD)$(git diff --quiet HEAD || echo '-dirty')"
code=$(git rev-list --count HEAD)
# The screens (src/android) go into the APK: built first.
prep=""
[ "$target" = apk ] && prep="npm run build --silent >/dev/null && "
exec scripts/remote.sh "${prep}rtk make -C android $target VERSION=$version VERSION_CODE=$code APP_HOST=${APP_HOST:-} CLEARTEXT=${CLEARTEXT:-false}"
