#!/usr/bin/env bash
# Sync the working tree to dev-build and run a command there.
#
#   scripts/remote.sh                 # gofmt -l, build, vet, test -race
#   scripts/remote.sh web             # npm ci, typecheck, test, build, the sinks check
#   scripts/remote.sh all             # web, then the Go checks (they see the bundles)
#   scripts/remote.sh <command ...>   # run <command> in the remote tree
#
# dev-build is authoritative: a pass there is the only pass that counts.
# The editing machine has neither Go nor Node; everything builds there.
set -euo pipefail

REMOTE_HOST=${REMOTE_HOST:-guillaume@10.0.50.21}
REMOTE_DIR=${REMOTE_DIR:-agendouille}

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# scripts/rsync-excludes keeps the database and agendouille.yaml here.
rsync -a --delete --exclude-from="$repo_root/scripts/rsync-excludes" \
	"$repo_root/" "$REMOTE_HOST:$REMOTE_DIR/"

# Single quotes on purpose: these expand on dev-build, not here. rtk trims
# the build noise and passes errors and warnings through.
# shellcheck disable=SC2016
go_checks='out=$(gofmt -l .); if [ -n "$out" ]; then echo "not gofmt-ed:"; echo "$out"; exit 1; fi;
rtk go build ./... && rtk go vet ./... && rtk go test -race ./...'
web_checks='rtk npm ci && rtk npm run typecheck && rtk npm test && rtk npm run build && scripts/check-sinks.sh'

case "${1:-}" in
"") set -- "$go_checks" ;;
web) set -- "$web_checks" ;;
all) set -- "$web_checks && $go_checks" ;;
esac

# shellcheck disable=SC2029
ssh "$REMOTE_HOST" "cd $REMOTE_DIR && $*"
