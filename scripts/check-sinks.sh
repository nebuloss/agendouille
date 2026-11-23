#!/usr/bin/env sh
# Fails if src/ uses a DOM sink that parses HTML. Event titles and names come
# from every family member's phone, so text reaches the DOM through
# textContent only.
set -eu

cd "$(dirname "$0")/.."

pattern='innerHTML|outerHTML|insertAdjacentHTML|document\.write|createContextualFragment|setHTMLUnsafe|parseHTMLUnsafe|DOMParser'

if grep -rnE "$pattern" src/; then
	echo "forbidden HTML sink in src/ (see PLAN.md 9.2)" >&2
	exit 1
fi
echo "no HTML sinks in src/"
