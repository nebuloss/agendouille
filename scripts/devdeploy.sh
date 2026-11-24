#!/usr/bin/env bash
# Build the working tree on dev-build and install it on a server, without
# GitHub: a release takes minutes, this takes seconds.
#
#   scripts/devdeploy.sh home                       # scripts/conf/targets/home.conf
#   PVE=root@host CT=408 scripts/devdeploy.sh       # a Proxmox container, via pct
#   TARGET=user@host scripts/devdeploy.sh           # any host, via ssh
#
# dev-build makes the package (scripts/build/package.sh: both bundles and the
# Android app in one binary, deploy/ beside it); deploy/install.sh installs
# it there, so the same command does a first install and every upgrade. It
# never touches /etc/agendouille.yaml, the data or an admin password that
# exist; on a first install it prints the admin password here, once.
# AGENDOUILLE_INSTALL_* in the environment are passed on (lib/target.sh).
#
# Settings: scripts/conf/build.conf (the build host, its trees, the
# architecture); APP_HOST, the public host for the app's App Links, from the
# target or the environment.
set -euo pipefail
# shellcheck source=SCRIPTDIR/lib/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
# shellcheck source=SCRIPTDIR/lib/target.sh
. "$AGD_ROOT/scripts/lib/target.sh"
load_conf "$AGD_CONF/build.conf"
resolve_target "${1:-}"

version=$(agd_version)
code=$(agd_version_code)
remote_pkg=/tmp/agendouille-devdeploy.tgz

say "syncing to $BUILD_HOST:$DEPLOY_DIR"
agd_sync "$BUILD_HOST" "$DEPLOY_DIR"

say "building $version (code $code) on $BUILD_HOST"
# shellcheck disable=SC2029  # expanded here on purpose
ssh "$BUILD_HOST" "cd $DEPLOY_DIR && bash scripts/build/package.sh $version $code '${APP_HOST:-}' $remote_pkg"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
scp -q "$BUILD_HOST:$remote_pkg" "$tmp/pkg.tgz"
# shellcheck disable=SC2029
ssh "$BUILD_HOST" "rm -f $remote_pkg"

say "installing on $(target_label)"
push_to_target "$tmp/pkg.tgz" /tmp/agendouille-pkg.tgz
on_target "set -e; rm -rf /tmp/agendouille-pkg; mkdir -p /tmp/agendouille-pkg
tar -C /tmp/agendouille-pkg -xzf /tmp/agendouille-pkg.tgz
env AGENDOUILLE_INSTALL_BINARY=/tmp/agendouille-pkg/agendouille$(install_knobs) sh /tmp/agendouille-pkg/install.sh
rm -rf /tmp/agendouille-pkg /tmp/agendouille-pkg.tgz"
