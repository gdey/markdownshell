#!/bin/sh
# run.sh builds the bridge and runs the app from the package, for
# working on it. Arguments go to mdsh-swift.
#
#   scripts/run.sh [file.md | folder]
set -eu
cd "$(dirname "$0")/.."
(cd bridge && go build -o ../.build/mdsh-swift-bridge .)
MDSH_SWIFT_BRIDGE="$PWD/.build/mdsh-swift-bridge" exec swift run mdsh-swift "$@"
