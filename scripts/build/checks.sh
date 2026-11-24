#!/usr/bin/env bash
# The checks, on the build host, in a synced tree (scripts/remote.sh runs
# them). rtk trims the build noise and passes errors and warnings through.
#
#   bash scripts/build/checks.sh web   # npm ci, typecheck, test, build, the sinks check
#   bash scripts/build/checks.sh go    # gofmt -l, build, vet, test -race
#   bash scripts/build/checks.sh web go
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

web() {
	rtk npm ci
	rtk npm run typecheck
	rtk npm test
	rtk npm run build
	scripts/check-sinks.sh
}

go_checks() {
	local out
	out=$(gofmt -l .)
	if [ -n "$out" ]; then
		echo "not gofmt-ed:"
		echo "$out"
		exit 1
	fi
	rtk go build ./...
	rtk go vet ./...
	rtk go test -race ./...
}

[ $# -gt 0 ] || set -- go
for what in "$@"; do
	case $what in
	web) web ;;
	go) go_checks ;;
	*)
		echo "checks.sh: web or go, not $what" >&2
		exit 2
		;;
	esac
done
