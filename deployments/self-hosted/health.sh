#!/bin/sh
set -eu
# Keep the proxy key out of process arguments and logs.
key=$(cat "$API_PROXY_SECRET_FILE")
case "$key" in *[!0-9a-fA-F]*|'') exit 1;; esac
[ "${#key}" -eq 64 ]
printf 'header = "X-Maker-Proxy-Key: %s"\n' "$key" |
    curl --config - --fail --silent --output /dev/null --max-time 3 \
    -H "Host: ${API_PUBLIC_ORIGIN#https://}" -H 'X-Forwarded-Proto: https' \
    http://127.0.0.1:8080/health/ready
# Verify the proxy listener is alive; authentication remains required.
[ "$(curl -k --silent --output /dev/null --write-out '%{http_code}' --max-time 3 \
    --connect-to "::127.0.0.1:8443" "$API_PUBLIC_ORIGIN/")" = 401 ]
