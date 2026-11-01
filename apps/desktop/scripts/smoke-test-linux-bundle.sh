#!/usr/bin/env bash

set -euo pipefail

archive_path="${1:-}"

if [[ -z "$archive_path" ]]; then
	echo "usage: $0 <linux-desktop-archive.tar.gz>"
	exit 1
fi

if [[ ! -f "$archive_path" ]]; then
	echo "archive not found: $archive_path"
	exit 1
fi

if curl --noproxy '*' --connect-timeout 1 --max-time 2 -sf http://127.0.0.1:8080/app-config >/dev/null 2>&1; then
	echo "localhost:8080 is already serving /app-config before smoke test"
	exit 1
fi

work_dir="$(mktemp -d)"
runtime_dir="$work_dir/runtime"
extract_dir="$work_dir/extracted"
log_file="$work_dir/app.log"
response_file="$work_dir/app-config.json"
binary_path=""
app_pid=""

cleanup() {
	if [[ -n "$app_pid" ]] && kill -0 "$app_pid" 2>/dev/null; then
		kill "$app_pid" 2>/dev/null || true
		for ((attempt = 0; attempt < 25; attempt++)); do
			if ! kill -0 "$app_pid" 2>/dev/null; then
				break
			fi
			sleep 0.2
		done
		if kill -0 "$app_pid" 2>/dev/null; then
			kill -KILL "$app_pid" 2>/dev/null || true
		fi
		wait "$app_pid" 2>/dev/null || true
	fi
	rm -rf "$work_dir"
}

trap cleanup EXIT

mkdir -p "$runtime_dir" "$extract_dir"
echo "Extracting packaged application"
tar -xzf "$archive_path" -C "$extract_dir"

license_notice="$(find "$extract_dir" -type f -name 'THIRD_PARTY_FONTS_OFL-1.1.txt' | head -n 1)"
if [[ -z "$license_notice" || ! -s "$license_notice" ]]; then
	echo "font license notice missing from packaged application"
	exit 1
fi

binary_path="$(find "$extract_dir" -type f -name 'Maker Track Studio' | head -n 1)"

if [[ -z "$binary_path" ]]; then
	echo "desktop binary not found in archive"
	exit 1
fi

echo "Checking packaged build metadata"
metadata="$(timeout 10s "$binary_path" --version)"
node -e '
const info = JSON.parse(process.argv[1]);
if (info.product !== "Maker Track Studio" ||
    (process.env.BUILD_VERSION && info.version !== process.env.BUILD_VERSION) ||
    (process.env.BUILD_COMMIT && info.commit !== process.env.BUILD_COMMIT)) {
    throw new Error("packaged build metadata mismatch");
}
' "$metadata"

export SQLITE_PATH="$runtime_dir/app.db"

if [[ -z "${DISPLAY:-}" ]] && command -v xvfb-run >/dev/null 2>&1; then
	xvfb-run -a "$binary_path" >"$log_file" 2>&1 &
else
	"$binary_path" >"$log_file" 2>&1 &
fi

app_pid=$!
echo "Starting packaged application; waiting up to 30 seconds for the desktop API"

deadline=$((SECONDS + 30))

until curl --noproxy '*' --connect-timeout 1 --max-time 2 -sf http://127.0.0.1:8080/app-config >"$response_file"; do
	if ! kill -0 "$app_pid" 2>/dev/null; then
		echo "packaged application exited before the desktop API responded"
		[[ -f "$log_file" ]] && cat "$log_file"
		exit 1
	fi
	if (( SECONDS >= deadline )); then
		echo "desktop API did not respond within 30 seconds"
		[[ -f "$log_file" ]] && cat "$log_file"
		exit 1
	fi
	sleep 1
done

if [[ ! -s "$response_file" ]]; then
	echo "desktop API returned an empty response"
	[[ -f "$log_file" ]] && cat "$log_file"
	exit 1
fi

node -e 'JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"))' "$response_file"

echo "Packaged application responded with valid app configuration"
cat "$response_file"
