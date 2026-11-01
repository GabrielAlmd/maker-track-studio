#!/usr/bin/env bash
set -euo pipefail
report=${1:?Pass a report path prefix}
shift
mkdir -p "$(dirname "$report")"
# JSON mode is evidence output only: it does not fail on vulnerable symbols.
govulncheck -json "$@" ./... > "$report.json"
# The text handler supplies the vulnerability exit code without a second scan.
govulncheck -mode=convert < "$report.json" | tee "$report.txt"
