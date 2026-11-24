# shellcheck shell=bash
# What every script shares: where the repository is, its configuration
# (scripts/conf), messages, the build's version, the sync to the build host
# and the seeding of a test server. Sourced, never run:
#
#   . "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

# shellcheck disable=SC2034  # used by the scripts that source this
{
	AGD_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
	AGD_CONF="$AGD_ROOT/scripts/conf"
	AGD_FIXTURES="$AGD_ROOT/scripts/fixtures"
}

say() { printf '> %s\n' "$*"; }
die() {
	printf '%s: %s\n' "$(basename "$0")" "$*" >&2
	exit 1
}

# load_conf FILE: its KEY=value lines, taken literally (nothing expands) and
# exported. The environment wins: a variable already set is kept.
load_conf() {
	local file=$1 line key value
	[ -f "$file" ] || die "no configuration at ${file#"$AGD_ROOT"/}"
	while IFS= read -r line || [ -n "$line" ]; do
		case $line in '' | '#'*) continue ;; esac
		key=${line%%=*}
		value=${line#*=}
		[[ $key =~ ^[A-Z_][A-Z0-9_]*$ ]] || die "${file#"$AGD_ROOT"/}: not KEY=value: $line"
		[ -n "${!key:-}" ] || printf -v "$key" '%s' "$value"
		export "${key?}"
	done <"$file"
}

# The build's version: dev-<commit>, -dirty with changes not committed.
agd_version() {
	printf 'dev-%s%s\n' "$(git -C "$AGD_ROOT" rev-parse --short HEAD)" "$(git -C "$AGD_ROOT" diff --quiet HEAD || echo -dirty)"
}

# The Android version code: the commit count, as releases count it, so the
# later build always installs over the earlier one.
agd_version_code() {
	git -C "$AGD_ROOT" rev-list --count HEAD
}

# agd_sync HOST DIR: the working tree to the build host, without what
# scripts/conf/rsync-excludes keeps here (the database, the configuration).
agd_sync() {
	rsync -az --delete --exclude-from="$AGD_CONF/rsync-excludes" "$AGD_ROOT/" "$1:$2/"
}

# agd_seed BINARY FILE: runs each line of FILE as the arguments of the
# agendouille binary (shell words; variables of the caller expand, so a
# seed can name a file the caller made). Blank lines and # comments skipped.
agd_seed() {
	local bin=$1 file=$2 line
	while IFS= read -r line || [ -n "$line" ]; do
		case $line in '' | '#'*) continue ;; esac
		eval "\"\$bin\" $line" >/dev/null || die "seed ${file#"$AGD_ROOT"/}: failed: $line"
	done <"$file"
}

# agd_week TEMPLATE OUT MONDAY: an iCalendar fixture of this week. @D0@ to
# @D6@ become the days from MONDAY (YYYY-MM-DD) as YYYYMMDD, and lines end
# in CRLF, as RFC 5545 wants.
agd_week() {
	local template=$1 out=$2 monday=$3 n args=()
	for n in 0 1 2 3 4 5 6; do
		args+=(-e "s/@D$n@/$(date -d "$monday +$n days" +%Y%m%d)/g")
	done
	sed "${args[@]}" -e 's/$/\r/' "$template" >"$out"
}

# agd_wait URL: until it answers (5 s at most).
agd_wait() {
	local _
	for _ in $(seq 50); do
		curl -fsS "$1" >/dev/null 2>&1 && return 0
		sleep 0.1
	done
	die "nothing answers at $1"
}
