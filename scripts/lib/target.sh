# shellcheck shell=bash
# Where a deploy goes and how to reach it: a Proxmox container (PVE and CT,
# through pct) or a host (TARGET, through ssh), named by a file of
# scripts/conf/targets or given in the environment. Sourced after
# lib/common.sh.

# resolve_target [NAME]: loads scripts/conf/targets/NAME.conf when named,
# then checks that the target is one or the other. No default on purpose:
# guessing which box to restart is how the wrong one gets restarted.
resolve_target() {
	if [ -n "${1:-}" ]; then
		[[ $1 =~ ^[a-z0-9_-]+$ ]] || die "not a target name: $1"
		load_conf "$AGD_CONF/targets/$1.conf"
	fi
	PVE=${PVE:-} CT=${CT:-} TARGET=${TARGET:-}
	if [ -n "$PVE" ] || [ -n "$CT" ]; then
		if [ -z "$PVE" ] || [ -z "$CT" ]; then die "PVE and CT go together: PVE=root@host CT=<id>"; fi
		[ -z "$TARGET" ] || die "set PVE and CT, or TARGET, not both"
	elif [ -z "$TARGET" ]; then
		local f names=""
		for f in "$AGD_CONF"/targets/*.conf; do
			[ -e "$f" ] || continue
			f=${f##*/}
			names="$names ${f%.conf}"
		done
		die "say where: a target (${names# }), PVE=root@host CT=<id>, or TARGET=user@host"
	fi
}

target_label() {
	if [ -n "$CT" ]; then echo "container $CT on $PVE"; else echo "$TARGET"; fi
}

# on_target COMMAND: runs a shell command there as root; stdin goes along.
on_target() {
	local quoted
	quoted=$(printf '%q' "$1")
	if [ -n "$CT" ]; then
		# shellcheck disable=SC2029  # expanded here on purpose
		ssh "$PVE" "pct exec $CT -- env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin sh -c $quoted"
	else
		local sudo=sudo
		[ "${TARGET%%@*}" != root ] || sudo=
		# shellcheck disable=SC2029
		ssh "$TARGET" "$sudo sh -c $quoted"
	fi
}

# push_to_target FILE PATH: a local file, to PATH there.
push_to_target() {
	if [ -n "$CT" ]; then
		ssh "$PVE" 'cat >/tmp/agendouille-push' <"$1"
		# shellcheck disable=SC2029
		ssh "$PVE" "pct push $CT /tmp/agendouille-push $2 && rm -f /tmp/agendouille-push"
	else
		scp -q "$1" "$TARGET:$2"
	fi
}

# install_knobs: install.sh's first-install settings, as env words.
install_knobs() {
	local k
	for k in AGENDOUILLE_INSTALL_ADMIN_LISTEN AGENDOUILLE_INSTALL_PUBLIC_URL AGENDOUILLE_INSTALL_PUBLIC_LISTEN; do
		[ -z "${!k:-}" ] || printf ' %s=%s' "$k" "${!k}"
	done
}
