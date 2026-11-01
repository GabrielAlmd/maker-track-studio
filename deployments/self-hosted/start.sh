#!/bin/sh
set -eu
umask 077
selfhostctl prepare
nginx -e stderr -t -c /tmp/nginx/nginx.conf
maker-api &
api=$!
nginx -e stderr -c /tmp/nginx/nginx.conf -g 'daemon off;' &
proxy=$!
selfhostctl renew &
renew=$!
stop() {
    trap '' TERM INT
    kill -TERM "$api" "$renew" 2>/dev/null || true
    kill -QUIT "$proxy" 2>/dev/null || true
    (sleep 20; kill -KILL "$api" "$proxy" "$renew" 2>/dev/null || true) &
    watchdog=$!
    wait "$api" 2>/dev/null || true
    wait "$proxy" 2>/dev/null || true
    wait "$renew" 2>/dev/null || true
    kill "$watchdog" 2>/dev/null || true
}
trap 'stop; exit 0' TERM INT
while kill -0 "$api" 2>/dev/null && kill -0 "$proxy" 2>/dev/null && kill -0 "$renew" 2>/dev/null; do
    sleep 1 &
    wait $! || true
done
stop
exit 1
