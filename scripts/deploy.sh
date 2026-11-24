#!/usr/bin/env bash
# Install a GitHub release of agendouille on a server: the same install.sh
# as the one-line install, from the release's tag, which downloads that
# release's binary and checks it against its SHA256SUMS.
#
#   scripts/deploy.sh home v0.2.0                  # scripts/conf/targets/home.conf
#   scripts/deploy.sh home                         # the latest release
#   PVE=root@host CT=408 scripts/deploy.sh '' v0.2.0
#   TARGET=root@host scripts/deploy.sh
#
# A release is what CI built and signed (.github/workflows/release.yml):
# the server, and the Android app embedded in it and published beside it,
# which the app updates itself from. For a build of the working tree,
# without GitHub, there is scripts/devdeploy.sh.
#
# install.sh is idempotent: a first install prints the admin password here,
# once; an upgrade never touches /etc/agendouille.yaml, the data or the
# password. AGENDOUILLE_INSTALL_* in the environment are passed on.
set -euo pipefail
# shellcheck source=SCRIPTDIR/lib/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
# shellcheck source=SCRIPTDIR/lib/target.sh
. "$AGD_ROOT/scripts/lib/target.sh"
resolve_target "${1:-}"

version=${2:-latest}
if [ "$version" = latest ]; then
	installer="$AGD_ROOT/deploy/install.sh"
else
	case $version in v*) ;; *) version="v$version" ;; esac
	git -C "$AGD_ROOT" fetch -q --tags origin 2>/dev/null || true
	installer=$(mktemp)
	trap 'rm -f "$installer"' EXIT
	git -C "$AGD_ROOT" show "$version:deploy/install.sh" >"$installer" || die "no tag $version: git fetch --tags, or check the name"
fi

say "installing agendouille $version on $(target_label)"
on_target "env AGENDOUILLE_INSTALL_VERSION=$version$(install_knobs) sh -s" <"$installer"
