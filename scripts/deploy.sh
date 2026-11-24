#!/bin/sh
# Install a GitHub release of agendouille on a server: the same install.sh
# as the one-line install, from the release's tag, which downloads that
# release's binary and checks it against its SHA256SUMS.
#
#   PVE=root@10.0.0.2 CT=408 scripts/deploy.sh v0.7.0   # a Proxmox container, via pct
#   TARGET=root@host scripts/deploy.sh                  # any host, via ssh; the latest release
#
# A release is what CI built and signed (.github/workflows/release.yml):
# the server, and the Android app embedded in it and published beside it,
# which the app updates itself from. For a build of the working tree,
# without GitHub, there is scripts/devdeploy.sh.
#
# install.sh is idempotent: a first install prints the admin password here,
# once; an upgrade never touches /etc/agendouille.yaml, the data or the
# password. Knobs for a first install are passed on:
# AGENDOUILLE_INSTALL_ADMIN_LISTEN, AGENDOUILLE_INSTALL_PUBLIC_URL.
set -eu

PVE=${PVE:-}
CT=${CT:-}
TARGET=${TARGET:-}

die() { printf 'deploy: %s\n' "$*" >&2; exit 1; }

# No default target on purpose, as devdeploy.sh: guessing which box to
# restart is how the wrong one gets restarted.
if [ -n "$PVE" ] || [ -n "$CT" ]; then
	[ -n "$PVE" ] && [ -n "$CT" ] || die "PVE and CT go together: PVE=root@host CT=<id>"
	[ -z "$TARGET" ] || die "set PVE and CT, or TARGET, not both"
elif [ -z "$TARGET" ]; then
	die "say where to deploy: PVE=root@host CT=<id> (Proxmox container) or TARGET=user@host"
fi

version=${1:-latest}
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
if [ "$version" = latest ]; then
	installer=$(cat "$root/deploy/install.sh")
else
	case $version in v*) ;; *) version="v$version" ;; esac
	git -C "$root" fetch -q --tags origin 2>/dev/null || true
	installer=$(git -C "$root" show "$version:deploy/install.sh") || die "no tag $version here: git fetch --tags, or check the name"
fi

env="AGENDOUILLE_INSTALL_VERSION=$version"
[ -z "${AGENDOUILLE_INSTALL_ADMIN_LISTEN:-}" ] || env="$env AGENDOUILLE_INSTALL_ADMIN_LISTEN=$AGENDOUILLE_INSTALL_ADMIN_LISTEN"
[ -z "${AGENDOUILLE_INSTALL_PUBLIC_URL:-}" ] || env="$env AGENDOUILLE_INSTALL_PUBLIC_URL=$AGENDOUILLE_INSTALL_PUBLIC_URL"

echo "> installing agendouille $version on ${TARGET:-$PVE (container $CT)}"
if [ -n "$CT" ]; then
	# shellcheck disable=SC2029  # expanded here on purpose
	printf '%s\n' "$installer" | ssh "$PVE" "pct exec $CT -- env $env sh -s"
else
	# shellcheck disable=SC2029
	printf '%s\n' "$installer" | ssh "$TARGET" "env $env sh -s"
fi
