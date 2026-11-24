#!/usr/bin/env bash
# Build agendouille on dev-build and install it on the home server, without
# GitHub: a release takes minutes, this takes seconds.
#
#   PVE=root@10.0.0.2 CT=408 scripts/devdeploy.sh   # a Proxmox container, via pct
#   TARGET=user@host scripts/devdeploy.sh            # any host, via ssh
#
# It copies the binary and deploy/ there and runs deploy/install.sh with
# that binary, so the same command does a first install and every upgrade.
# install.sh never touches /etc/agendouille.yaml, the data or an admin
# password that exist; on a first install it prints the admin password
# here, once. Knobs passed to install.sh on a first install:
# AGENDOUILLE_INSTALL_ADMIN_LISTEN, AGENDOUILLE_INSTALL_PUBLIC_URL.
#
# Others: BUILD_HOST (default guillaume@10.0.50.21), REMOTE_DIR (the build
# tree on dev-build, default agendouille-deploy), GOARCH (amd64), GOARM (7),
# APP_HOST (the public host, for the Android app's App Links; without it
# the app opens enrollment links through agendouille:// only).
#
# The Android app is built too, with the same version, and embedded in the
# server (PLAN.md 7.6), when the build host has the SDK (android/Makefile).
set -euo pipefail

BUILD_HOST=${BUILD_HOST:-guillaume@10.0.50.21}
REMOTE_DIR=${REMOTE_DIR:-agendouille-deploy}
GOARCH=${GOARCH:-amd64}
GOARM=${GOARM:-7}
PVE=${PVE:-}
CT=${CT:-}
TARGET=${TARGET:-}
APP_HOST=${APP_HOST:-}

say() { printf '> %s\n' "$*"; }
die() { printf 'devdeploy: %s\n' "$*" >&2; exit 1; }

# No default target on purpose: guessing which box to restart is how the
# wrong one gets restarted.
if [ -n "$PVE" ] || [ -n "$CT" ]; then
	[ -n "$PVE" ] && [ -n "$CT" ] || die "PVE and CT go together: PVE=root@host CT=<id>"
	[ -z "$TARGET" ] || die "set PVE and CT, or TARGET, not both"
elif [ -z "$TARGET" ]; then
	die "say where to deploy: PVE=root@host CT=<id> (Proxmox container) or TARGET=user@host"
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"
version="dev-$(git rev-parse --short HEAD)$(git diff --quiet HEAD || echo '-dirty')"
code=$(git rev-list --count HEAD)

say "syncing to $BUILD_HOST:$REMOTE_DIR"
rsync -az --delete --exclude-from=scripts/rsync-excludes ./ "$BUILD_HOST:$REMOTE_DIR/"

say "building $version for linux/$GOARCH (both bundles embedded)"
# shellcheck disable=SC2029
ssh "$BUILD_HOST" "cd $REMOTE_DIR && set -e
	if [ ! -d node_modules ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then npm ci --silent; fi
	npm run build --silent >/dev/null
	test -s internal/web/assets/static/app/app.js
	test -s internal/web/assets/static/admin/admin.js
	if [ -d \$HOME/android-build/sdk ]; then
		# Signed with the release key when the build host has it: the app then
		# installs over a release's (and a release over it), its version code
		# deciding. Without it, the debug key.
		sign=''
		if [ -f \$HOME/android-build/agendouille-release.keystore ]; then
			sign=\"KEYSTORE=\$HOME/android-build/agendouille-release.keystore KS_PASS=\$(cat \$HOME/android-build/agendouille-release.password) KEY_ALIAS=agendouille\"
		fi
		make -s -C android apk VERSION=$version VERSION_CODE=$code APP_HOST=$APP_HOST \$sign >/dev/null
		cp android/build/agendouille.apk internal/web/assets/apk/agendouille.apk
		make -s -C android cert \$sign >internal/web/assets/apk/cert.txt
		echo $version >internal/web/assets/apk/version.txt
		echo '> Android app embedded'
	else
		echo '> no Android SDK on the build host: no app embedded'
	fi
	CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH GOARM=$GOARM go build -trimpath \
		-ldflags='-s -w -X main.version=$version' -o /tmp/agendouille-devdeploy ./cmd/agendouille"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/pkg"
rsync -az "$BUILD_HOST:/tmp/agendouille-devdeploy" "$tmp/pkg/agendouille"
# shellcheck disable=SC2029
ssh "$BUILD_HOST" "rm -f /tmp/agendouille-devdeploy"
cp deploy/install.sh deploy/agendouille.service deploy/agendouille.openrc \
	deploy/agendouille-container.conf deploy/agendouille.yaml.example "$tmp/pkg/"
tar -C "$tmp/pkg" -czf "$tmp/pkg.tgz" .

# Run there. The knobs travel as plain words: no quotes inside.
knobs="AGENDOUILLE_INSTALL_BINARY=/tmp/agendouille-pkg/agendouille"
[ -z "${AGENDOUILLE_INSTALL_ADMIN_LISTEN:-}" ] || knobs="$knobs AGENDOUILLE_INSTALL_ADMIN_LISTEN=$AGENDOUILLE_INSTALL_ADMIN_LISTEN"
[ -z "${AGENDOUILLE_INSTALL_PUBLIC_URL:-}" ] || knobs="$knobs AGENDOUILLE_INSTALL_PUBLIC_URL=$AGENDOUILLE_INSTALL_PUBLIC_URL"
post="set -e; rm -rf /tmp/agendouille-pkg; mkdir -p /tmp/agendouille-pkg; tar -C /tmp/agendouille-pkg -xzf /tmp/agendouille-pkg.tgz; \
env $knobs sh /tmp/agendouille-pkg/install.sh; rm -rf /tmp/agendouille-pkg /tmp/agendouille-pkg.tgz"

if [ -n "$PVE" ]; then
	say "installing in container $CT on $PVE"
	ssh "$PVE" 'cat > /tmp/agendouille-pkg.tgz' <"$tmp/pkg.tgz"
	# shellcheck disable=SC2029
	ssh "$PVE" "pct push $CT /tmp/agendouille-pkg.tgz /tmp/agendouille-pkg.tgz && rm -f /tmp/agendouille-pkg.tgz && \
		pct exec $CT -- env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin sh -c '$post'"
else
	say "installing on $TARGET"
	sudo=sudo
	[ "${TARGET%%@*}" != root ] || sudo=
	scp -q "$tmp/pkg.tgz" "$TARGET:/tmp/agendouille-pkg.tgz"
	# shellcheck disable=SC2029
	ssh "$TARGET" "$sudo sh -c '$post'"
fi
