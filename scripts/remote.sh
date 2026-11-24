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
# The host and the tree: scripts/conf/build.conf (BUILD_HOST, REMOTE_DIR).
set -euo pipefail
# shellcheck source=SCRIPTDIR/lib/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
load_conf "$AGD_CONF/build.conf"

agd_sync "$BUILD_HOST" "$REMOTE_DIR"

case "${1:-}" in
"") set -- bash scripts/build/checks.sh go ;;
web) set -- bash scripts/build/checks.sh web ;;
all) set -- bash scripts/build/checks.sh web go ;;
esac

# shellcheck disable=SC2029  # the command runs there, as given
ssh "$BUILD_HOST" "cd $REMOTE_DIR && $*"
