#!/bin/sh
# Install or upgrade agendouille as a system service.
#
#   curl -fsSL https://raw.githubusercontent.com/nebuloss/agendouille/main/deploy/install.sh | sudo sh
#
# or, from a checkout (it then builds from source, so what gets installed
# is what you are looking at):
#
#   sudo deploy/install.sh
#
# Piped from curl it downloads the release binary for this machine and checks
# it against the published SHA256SUMS, refusing anything it cannot verify.
#
# IDEMPOTENT: run it again to upgrade. It never overwrites
# /etc/agendouille.yaml, the data directory or an admin password once they
# exist.
#
# Knobs, read only on a first install (afterwards the file and the admin UI
# win):
#   AGENDOUILLE_INSTALL_ADMIN_LISTEN  where the admin UI listens. Default
#       127.0.0.1:8792; give 0.0.0.0:8792 to reach it from the LAN (it is
#       then plain http, which the LAN-only admin UI accepts).
#   AGENDOUILLE_INSTALL_PUBLIC_URL    the address the family uses, the DMZ
#       proxy's (https://agenda.example.org). With it, the public listener
#       (CalDAV and the web app) is turned on at :8791.
#   AGENDOUILLE_INSTALL_VERSION / _REPO   which release (default: the latest).
#
# POSIX sh on purpose: a fresh Alpine container has busybox, not bash.
set -eu

REPO=${AGENDOUILLE_INSTALL_REPO:-nebuloss/agendouille}
VERSION=${AGENDOUILLE_INSTALL_VERSION:-latest}
ADMIN_LISTEN=${AGENDOUILLE_INSTALL_ADMIN_LISTEN:-}
PUBLIC_URL=${AGENDOUILLE_INSTALL_PUBLIC_URL:-}
PUBLIC_LISTEN=${AGENDOUILLE_INSTALL_PUBLIC_LISTEN:-0.0.0.0:8791}

BINARY=/usr/local/bin/agendouille
CONFIG=/etc/agendouille.yaml
UNIT=/etc/systemd/system/agendouille.service
DROPIN_DIR=/etc/systemd/system/agendouille.service.d
OPENRC=/etc/init.d/agendouille
SERVICE_USER=agendouille

say() { printf '> %s\n' "$*"; }
warn() { printf '! %s\n' "$*" >&2; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run this as root, or through sudo"
[ "$(uname -s)" = Linux ] || die "this installer is for Linux"

# --- where are we? -----------------------------------------------------------

# A checkout is recognised by its own files rather than by $0, which is not a
# usable path when the script arrives on a pipe.
repo_root=""
case "$0" in
	*/*) candidate=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd || true) ;;
	*) candidate=$(pwd) ;;
esac
if [ -n "$candidate" ] && [ -f "$candidate/go.mod" ] && [ -d "$candidate/cmd/agendouille" ]; then
	repo_root=$candidate
fi
# devdeploy.sh hands over a binary it built: install that one.
PREBUILT=${AGENDOUILLE_INSTALL_BINARY:-}

case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	armv7* | armhf) arch=armv7 ;;
	*) [ -n "$repo_root$PREBUILT" ] || die "no release build for $(uname -m); build from a checkout instead" ;;
esac

# systemd, OpenRC, or neither (a container with no init).
INIT=none
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	INIT=systemd
elif command -v rc-update >/dev/null 2>&1 && [ -d /etc/init.d ]; then
	INIT=openrc
fi

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1" -o "$2"; }
	fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO "$2" "$1"; }
	fetch_stdout() { wget -qO- "$1"; }
else
	fetch() { die "neither curl nor wget is installed, so $1 cannot be downloaded"; }
	fetch_stdout() { fetch "$1" -; }
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM

# deploy_file NAME DEST copies deploy/NAME from beside this script, or
# downloads it from the release tag (falling back to main).
script_dir=$(cd "$(dirname "$0")" 2>/dev/null && pwd || true)
deploy_file() {
	if [ -n "$script_dir" ] && [ -f "$script_dir/$1" ]; then
		cp "$script_dir/$1" "$2"
	elif [ -n "$repo_root" ] && [ -f "$repo_root/deploy/$1" ]; then
		cp "$repo_root/deploy/$1" "$2"
	else
		fetch "https://raw.githubusercontent.com/$REPO/$VERSION/deploy/$1" "$2" 2>/dev/null ||
			fetch "https://raw.githubusercontent.com/$REPO/main/deploy/$1" "$2" ||
			die "cannot fetch deploy/$1"
	fi
}

# --- the binary --------------------------------------------------------------

install_from_source() {
	command -v go >/dev/null 2>&1 || die "go is not installed; run without a checkout to download a release"
	command -v npm >/dev/null 2>&1 || die "npm is not installed: the web app and the admin UI are built into the binary"
	say "building the two bundles"
	(cd "$repo_root" && { [ -e node_modules ] || npm ci --silent; } && npm run build --silent) ||
		die "the frontend build failed"
	if [ -n "${SUDO_UID:-}" ] && [ -n "${SUDO_GID:-}" ]; then
		chown -R "$SUDO_UID:$SUDO_GID" "$repo_root/internal/web/assets/static" "$repo_root/node_modules" 2>/dev/null || true
	fi
	git_c() { git -c safe.directory="$repo_root" -C "$repo_root" "$@"; }
	stamp=dev
	if command -v git >/dev/null 2>&1 && sha=$(git_c rev-parse --short HEAD 2>/dev/null); then
		stamp="dev-$sha"
		git_c diff --quiet HEAD 2>/dev/null || stamp="$stamp-dirty"
	fi
	say "building agendouille $stamp"
	(cd "$repo_root" && CGO_ENABLED=0 go build -trimpath \
		-ldflags="-s -w -X main.version=$stamp" -o "$TMP/agendouille" ./cmd/agendouille) ||
		die "the build failed"
}

install_from_release() {
	if [ "$VERSION" = latest ]; then
		say "asking GitHub for the latest release"
		VERSION=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" |
			sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
		[ -n "$VERSION" ] || die "cannot work out the latest version; set AGENDOUILLE_INSTALL_VERSION=vX.Y.Z"
	fi
	base="https://github.com/$REPO/releases/download/$VERSION"
	asset="agendouille-linux-$arch"
	say "downloading $asset ($VERSION)"
	fetch "$base/$asset" "$TMP/agendouille" || die "could not download $base/$asset"
	# This executable will run as a service, so what arrived has to be what
	# was built. Every way of not knowing that is a refusal.
	fetch "$base/SHA256SUMS" "$TMP/SHA256SUMS" ||
		die "could not fetch SHA256SUMS, so the download cannot be verified"
	command -v sha256sum >/dev/null 2>&1 ||
		die "sha256sum is not installed, so the download cannot be verified"
	want=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1; exit }' "$TMP/SHA256SUMS")
	[ -n "$want" ] || die "SHA256SUMS does not list $asset, so the download cannot be verified"
	got=$(sha256sum "$TMP/agendouille" | cut -d' ' -f1)
	[ "$got" = "$want" ] || die "checksum mismatch, refusing to install
  expected $want
  got      $got"
	say "checksum verified"
}

if [ -n "$PREBUILT" ]; then
	say "installing the binary at $PREBUILT"
	cp "$PREBUILT" "$TMP/agendouille"
elif [ -n "$repo_root" ]; then
	say "installing from the checkout at $repo_root"
	install_from_source
else
	say "installing the published release for linux/$arch"
	install_from_release
fi

chmod 0755 "$TMP/agendouille"
# Run it before installing it: a truncated download or a binary for the
# wrong architecture fails here, not as a service that will not start.
"$TMP/agendouille" version >/dev/null 2>&1 || die "the new binary will not run on this machine"

# --- account -----------------------------------------------------------------

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
	say "creating the $SERVICE_USER user"
	if command -v useradd >/dev/null 2>&1; then
		useradd --system --user-group --home-dir /var/lib/agendouille --no-create-home \
			--shell /usr/sbin/nologin "$SERVICE_USER"
	elif command -v adduser >/dev/null 2>&1; then
		# busybox: the group first and named, or the account lands in nogroup.
		addgroup -S "$SERVICE_USER" 2>/dev/null || true
		adduser -S -D -H -h /var/lib/agendouille -s /sbin/nologin -G "$SERVICE_USER" "$SERVICE_USER"
	else
		die "cannot create the $SERVICE_USER user: no useradd or adduser"
	fi
fi

# --- configuration -----------------------------------------------------------

host_ip=$(hostname -I 2>/dev/null | cut -d' ' -f1 || true)
if [ -z "$host_ip" ] && command -v ip >/dev/null 2>&1; then
	host_ip=$(ip -4 -o addr show scope global 2>/dev/null | awk '{ sub("/.*", "", $4); print $4; exit }')
fi

new_install=no
if [ -f "$CONFIG" ]; then
	say "$CONFIG exists, left alone"
else
	say "writing $CONFIG"
	new_install=yes
	deploy_file agendouille.yaml.example "$TMP/agendouille.yaml"
	if [ -n "$ADMIN_LISTEN" ]; then
		printf '\nadmin_listen: %s\n' "$ADMIN_LISTEN" >>"$TMP/agendouille.yaml"
		admin_host=${ADMIN_LISTEN%:*}
		case "$admin_host" in
		127.* | localhost | "[::1]") ;;
		*)
			# Off loopback the admin UI must say where it is reached; plain
			# http is accepted for a LAN-only admin UI, in so many words.
			url_host=$admin_host
			case "$url_host" in "" | 0.0.0.0 | "[::]" | "::") url_host=${host_ip:-127.0.0.1} ;; esac
			printf 'admin_public_url: http://%s:%s\nadmin_allow_insecure: true\n' "$url_host" "${ADMIN_LISTEN##*:}" >>"$TMP/agendouille.yaml"
			;;
		esac
	fi
	cp "$TMP/agendouille.yaml" "$CONFIG.new"
	mv "$CONFIG.new" "$CONFIG"
fi
# Re-applied on every run, so a file the service cannot read is repaired.
chown "root:$SERVICE_USER" "$CONFIG"
chmod 0640 "$CONFIG"

# The same check the service runs before starting: fail here, with the
# reason, rather than as a service that keeps restarting.
if ! effective=$("$TMP/agendouille" config -config "$CONFIG" 2>&1); then
	printf '%s\n' "$effective" | sed 's/^/  /' >&2
	die "$CONFIG is not usable (the reason is above); fix it and run this again"
fi
cfg_value() { printf '%s\n' "$effective" | sed -n "s/^$1: *//p" | sed -e 's/^"\(.*\)"$/\1/' -e "s/^'\(.*\)'\$/\1/" | head -n 1; }
data_dir=$(cfg_value data_dir)
admin_listen=$(cfg_value admin_listen)
admin_url=$(cfg_value admin_public_url)
[ -n "$admin_url" ] || admin_url="http://$admin_listen"

# 0700: the database holds the family's calendars.
install -d -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0700 "$data_dir"
[ "$data_dir" = /var/lib/agendouille ] ||
	warn "data_dir is $data_dir: the systemd unit only allows writes under /var/lib/agendouille (ReadWritePaths)"

# Stop before replacing: a restore staged by the admin UI is applied at the
# next start, and the database must not change under a running server.
stop_service() {
	case "$INIT" in
	systemd) systemctl stop agendouille 2>/dev/null || true ;;
	openrc) [ ! -e /run/openrc/softlevel ] || rc-service agendouille stop >/dev/null 2>&1 || true ;;
	esac
}
[ ! -x "$BINARY" ] || stop_service

# Written beside the old one and renamed: overwriting a running binary in
# place is "Text file busy", and a failed copy must leave the old one alone.
cp "$TMP/agendouille" "$BINARY.new"
chmod 0755 "$BINARY.new"
mv -f "$BINARY.new" "$BINARY"
say "installed $BINARY ($("$BINARY" version))"

# As the service user from here on: the database it creates must be its own.
as_service() {
	if command -v runuser >/dev/null 2>&1; then
		runuser -u "$SERVICE_USER" -- "$@"
	elif command -v su >/dev/null 2>&1; then
		# busybox su takes a shell command; the arguments here are ours.
		su -s /bin/sh "$SERVICE_USER" -c "$*"
	else
		"$@"
	fi
}

# --- the admin password ------------------------------------------------------

# The server refuses to start without one. It is made here, shown only on
# this terminal and kept only as a hash; --if-unset leaves one already set
# alone, so an upgrade never changes it.
admin_password=""
password=$(od -An -N18 -tx1 /dev/urandom | tr -d ' \n')
[ ${#password} -eq 36 ] || die "could not read 18 random bytes from /dev/urandom"
printf '%s\n' "$password" >"$TMP/pw"
chown "$SERVICE_USER" "$TMP" "$TMP/pw"
if ! passwd_out=$(as_service "$BINARY" admin passwd -config "$CONFIG" --if-unset <"$TMP/pw" 2>"$TMP/passwd.err"); then
	sed 's/^/  /' "$TMP/passwd.err" >&2
	die "could not set the admin password"
fi
rm -f "$TMP/pw"
case "$passwd_out" in
"Admin password set"*) admin_password=$password ;;
esac
unset password

# The public listener, on a first install with a public URL.
if [ "$new_install" = yes ] && [ -n "$PUBLIC_URL" ]; then
	insecure=false
	case "$PUBLIC_URL" in http://*) insecure=true ;; esac
	as_service "$BINARY" config -config "$CONFIG" set public_listen "$PUBLIC_LISTEN" public_url "$PUBLIC_URL" \
		public_allow_insecure "$insecure" >/dev/null || die "could not set the public listener"
	say "the public listener is on at $PUBLIC_LISTEN for $PUBLIC_URL"
fi
chown -R "$SERVICE_USER:$SERVICE_USER" "$data_dir"

# --- the service -------------------------------------------------------------

port=${admin_listen##*:}
probe_host=${admin_listen%:*}
case "$probe_host" in "" | 0.0.0.0 | "[::]" | "::") probe_host=127.0.0.1 ;; esac
health_url="http://$probe_host:$port/healthz"

healthy() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1
	elif command -v wget >/dev/null 2>&1; then
		wget -qO /dev/null -T 2 "$health_url" >/dev/null 2>&1
	else
		return 1
	fi
}

wait_healthy() {
	n=0
	while [ "$n" -lt "$1" ]; do
		healthy && return 0
		sleep 1
		n=$((n + 1))
	done
	return 1
}

started=no
case "$INIT" in
systemd)
	say "installing the systemd unit"
	deploy_file agendouille.service "$TMP/agendouille.service"
	install -m 0644 "$TMP/agendouille.service" "$UNIT"
	systemctl daemon-reload
	systemctl enable agendouille >/dev/null 2>&1
	systemctl restart agendouille || true
	if ! wait_healthy 15; then
		# An unprivileged container cannot grant the mount namespaces the
		# unit asks for: systemd fails it with 226/NAMESPACE. Strict first,
		# relaxed only on that.
		status=$(systemctl show -p ExecMainStatus --value agendouille 2>/dev/null || true)
		if [ "$status" = 226 ] || [ "$status" = 219 ]; then
			say "the hardened unit cannot start here ($status): installing the container drop-in"
			mkdir -p "$DROPIN_DIR"
			deploy_file agendouille-container.conf "$TMP/container.conf"
			install -m 0644 "$TMP/container.conf" "$DROPIN_DIR/container.conf"
			systemctl daemon-reload
			systemctl restart agendouille || true
			wait_healthy 15 || true
		fi
	fi
	if healthy; then
		started=yes
	else
		journalctl -u agendouille --no-pager --lines=25 2>/dev/null | sed 's/^/  /' >&2
		die "the service did not answer $health_url (its log is above)"
	fi
	;;
openrc)
	say "installing the OpenRC service"
	deploy_file agendouille.openrc "$TMP/agendouille.openrc"
	install -m 0755 "$TMP/agendouille.openrc" "$OPENRC"
	rc-update add agendouille default >/dev/null 2>&1 || true
	if [ -e /run/openrc/softlevel ]; then
		rc-service agendouille restart >/dev/null 2>&1 || rc-service agendouille start >/dev/null 2>&1 || true
		if wait_healthy 15; then
			started=yes
		else
			tail -n 25 /var/log/agendouille.log 2>/dev/null | sed 's/^/  /' >&2
			die "the service did not answer $health_url (its log is above)"
		fi
	else
		warn "OpenRC is installed but did not boot this system, so the service was not started."
	fi
	;;
*)
	warn "no init system here, so nothing was set up to start agendouille automatically."
	;;
esac

# --- what next ---------------------------------------------------------------

printf '\n'
if [ "$started" = yes ]; then
	say "$("$BINARY" version) is running: $health_url answers."
else
	say "Start it by hand, as the $SERVICE_USER user: $BINARY serve -config $CONFIG"
fi

if [ -n "$admin_password" ]; then
	cat <<EOF

+-- THE ADMIN PASSWORD -----------------------------------------------------
|
|   $admin_password
|
|  Shown once: only its hash is stored, and it is in no log.
|  Sign in at $admin_url/admin
|  Change it in the admin UI, or: $BINARY admin passwd -config $CONFIG
+--------------------------------------------------------------------------
EOF
fi

if [ "$new_install" = yes ]; then
	cat <<EOF

Next:

  1. Sign in at $admin_url/admin with the password above.
  2. Settings, Network: the public address the family uses (your proxy's
     https:// address) and the listener behind it, if not done here.
  3. Users: add the family, put them in groups, and give each a sign-in QR.

Behind a reverse proxy, and for the admin UI on the LAN only, see
deploy/README.md in the repository.
EOF
else
	say "Upgraded. $CONFIG, $data_dir and the admin password were left alone."
fi

case "$INIT" in
systemd) say "systemctl status agendouille / journalctl -u agendouille -f" ;;
openrc) say "rc-service agendouille status / tail -f /var/log/agendouille.log" ;;
esac
