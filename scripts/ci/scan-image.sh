#!/usr/bin/env bash
# Scan a final OCI archive without exposing Docker credentials or its daemon.
set -euo pipefail
archive=$(realpath "${1:?Pass an OCI archive}")
report_dir=$(realpath -m "${2:?Pass a report directory}")
arch=${3:?Pass amd64 or arm64}
case "$arch" in amd64|arm64) ;; *) exit 2 ;; esac
cache_dir=${TRIVY_CACHE_DIR:-${TMPDIR:-/tmp}/maker-trivy-cache}
mkdir -p "$report_dir" "$cache_dir"
cache_dir=$(realpath "$cache_dir")
# Trivy consumes an OCI layout directory; Buildx exports that layout as a tar.
layout=$(mktemp -d)
trap 'rm -rf "$layout"' EXIT
tar --no-same-owner --no-same-permissions -xf "$archive" -C "$layout"
# Permit the scanner's unprivileged UID to traverse its read-only layout.
chmod 755 "$layout"
# Trivy v0.76.1, pinned multi-platform manifest; do not replace with latest.
scanner=aquasec/trivy:0.76.1@sha256:790fd66d1697395d8693d1acaa1ea702406a20f109b6fc7bac8db623eca6c06c
printf '%s\n' "$scanner" > "$report_dir/scanner.txt"
sha256sum "$archive" > "$report_dir/archive.sha256"
scan_status=0
docker run --rm --user "$(id -u):$(id -g)" \
  -e TRIVY_CACHE_DIR=/cache \
  -v "$layout:/image:ro" -v "$report_dir:/reports" -v "$cache_dir:/cache" \
  "$scanner" image --input /image --platform "linux/$arch" \
  --scanners vuln --pkg-types os,library --severity HIGH,CRITICAL \
  --exit-code 1 --timeout 15m --format json --output /reports/vulnerabilities.json || scan_status=$?
# Keep the JSON evidence, but make a failed gate actionable in the job log too.
if [ -s "$report_dir/vulnerabilities.json" ]; then
  python3 - "$report_dir/vulnerabilities.json" <<'PYREPORT'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as report:
    results = json.load(report).get("Results", [])
count = 0
for target in results:
    for finding in target.get("Vulnerabilities") or []:
        count += 1
        fixed = finding.get("FixedVersion") or "no published fix"
        print(f"{finding['Severity']} {finding['VulnerabilityID']}: "
              f"{finding['PkgName']} {finding['InstalledVersion']} -> {fixed} "
              f"[{target['Target']}]")
print(f"Image security gate: {count} HIGH/CRITICAL findings")
PYREPORT
fi
exit "$scan_status"
