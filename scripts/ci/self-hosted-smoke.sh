#!/usr/bin/env bash
# Only disposable CI data; never uses installation projects or credentials.
set -euo pipefail
image=${1:?Pass a locally loaded candidate image tag}
baseline=${2:-}
root=$(mktemp -d)
project="maker-ci-${RANDOM}-${RANDOM}"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  printf 'evidence=%s/evidence\n' "$root" >> "$GITHUB_OUTPUT"
fi
mkdir "$root/evidence" "$root/secrets" "$root/snapshot"
chmod 700 "$root/snapshot"
cp deployments/self-hosted/compose.yml "$root/compose.yml"
export MAKER_TRACK_VERSION="ci-${GITHUB_RUN_ID:-local}"
export LAN_IP=127.0.0.1 HTTPS_PORT=8443 API_PUBLIC_ORIGIN=https://localhost:8443
compose=(docker compose --project-name "$project" --project-directory "$root")
phase=setup
cleanup() {
  status=$?
  trap - EXIT
  "${compose[@]}" logs --no-color > "$root/evidence/$phase.log" 2>&1 || true
  printf 'exit_status=%s\nlast_phase=%s\n' "$status" "$phase" >> "$root/evidence/result.txt"
  "${compose[@]}" down -v --timeout 30 || true
  # Private temporary files remain until the disposable CI runner is discarded.
  exit "$status"
}
trap cleanup EXIT
candidate_id=$(docker image inspect --format '{{.Id}}' "$image")
docker run --rm --entrypoint sh "$image" -c 'test -s /usr/share/licenses/maker-track/LICENSE && test -s /usr/share/licenses/maker-track/THIRD_PARTY_FONTS_OFL-1.1.txt'
first_image=$image
if [ -n "$baseline" ]; then
  first_image=$baseline
  baseline_id=$(docker image inspect --format '{{.Id}}' "$baseline")
  [ "$baseline_id" != "$candidate_id" ] || {
    echo 'Baseline and candidate must be different images for an upgrade test.' >&2
    exit 1
  }
  printf 'upgrade_rollback=requested\nbaseline=%s\n' "$baseline_id" > "$root/evidence/result.txt"
else
  printf 'upgrade_rollback=NOT_EXECUTED (no baseline supplied)\n' > "$root/evidence/result.txt"
fi
printf 'candidate=%s\n' "$candidate_id" >> "$root/evidence/result.txt"
docker image inspect --format 'architecture={{.Architecture}} os={{.Os}}' "$image" >> "$root/evidence/result.txt"
select_image() {
  docker tag "$1" "ghcr.io/gabrielalmd/maker-track-studio:$MAKER_TRACK_VERSION"
}
start() {
  "${compose[@]}" up -d --no-build --wait --wait-timeout 180
  id=$("${compose[@]}" ps -q studio)
  docker inspect "$id" | python3 -c 'import json,sys; p=json.load(sys.stdin)[0]["NetworkSettings"]["Ports"]; assert p.get("8080/tcp") is None; assert p["8443/tcp"]'
}
stop() {
  "${compose[@]}" stop --timeout 30
  test "$(docker inspect --format '{{.State.ExitCode}}' "$id")" = 0
  "${compose[@]}" logs --no-color > "$root/evidence/$phase.log"
}
printers() {
  "${request[@]}" --fail -u "maker:$password" "$API_PUBLIC_ORIGIN/api/printers" > "$1"
}
create_printer() {
  "${request[@]}" --fail -u "maker:$password" -H "Origin: $API_PUBLIC_ORIGIN" -H 'Content-Type: application/json' --data "{\"name\":\"$1\",\"manufacturer\":\"Test\",\"model\":\"Disposable\"}" "$API_PUBLIC_ORIGIN/api/printers" > /dev/null
}
select_image "$first_image"
key=$(openssl rand -hex 32)
password=$(openssl rand -hex 24)
printf '%s\n' "$key" > "$root/secrets/proxy.key"
hash=$(printf '%s\n' "$password" | docker run --rm -i --entrypoint selfhostctl "$image" hash-password)
printf '%s\n' "$hash" > "$root/secrets/auth.htpasswd"
chmod 755 "$root"
docker run --rm --user 0 --entrypoint sh -v "$root/secrets:/secrets" "$image" -c 'chown -R 10001:10001 /secrets; chmod 700 /secrets; chmod 600 /secrets/*'
phase=initial
start
"${compose[@]}" cp studio:/data/tls/root.crt "$root/root.crt"
request=(curl --silent --show-error --cacert "$root/root.crt")
test "$("${request[@]}" -o /dev/null -w '%{http_code}' "$API_PUBLIC_ORIGIN/api/printers")" = 401
create_printer 'CI persistence fixture'
printers "$root/printers.json"
"${request[@]}" --fail -u "maker:$password" "$API_PUBLIC_ORIGIN/api/desktop-data/export" > "$root/backup.zip"
python3 -m zipfile -t "$root/backup.zip"
"${request[@]}" --fail -u "maker:$password" -H "Origin: $API_PUBLIC_ORIGIN" -F "file=@$root/backup.zip" "$API_PUBLIC_ORIGIN/api/desktop-data/import" > /dev/null
printers "$root/restored.json"
cmp "$root/printers.json" "$root/restored.json"
stop
if [ -n "$baseline" ]; then
  # Snapshot all four volumes with both processes stopped, including private CA data.
  docker run --rm --user 0 --volumes-from "$id" --entrypoint sh -v "$root/snapshot:/snapshot" "$image" -c 'tar -cpf /snapshot/volumes.tar /workspace /diagnostics /backups /data/tls'
fi
"${compose[@]}" down --timeout 30
select_image "$image"
phase=candidate
start
printers "$root/restarted.json"
cmp "$root/printers.json" "$root/restarted.json"
if [ -n "$baseline" ]; then
  # A post-upgrade change must disappear after restoring the cold snapshot.
  create_printer 'CI post-upgrade fixture'
  stop
  # A separate maintenance container needs filesystem privileges that the app
  # deliberately drops. It sees only this test project's stopped volumes.
  docker run --rm --user 0 --volumes-from "$id" --entrypoint sh -v "$root/snapshot:/snapshot:ro" "$image" -c 'set -e; find /workspace /diagnostics /backups /data/tls -mindepth 1 -delete; tar -xpf /snapshot/volumes.tar -C /'
  "${compose[@]}" down --timeout 30
  select_image "$baseline"
  phase=rollback
  start
  printers "$root/rollback.json"
  cmp "$root/printers.json" "$root/rollback.json"
  printf 'upgrade_rollback=PASSED\n' >> "$root/evidence/result.txt"
fi
stop
printf 'compose_smoke=PASSED\n' >> "$root/evidence/result.txt"
cat "$root/evidence/result.txt"
