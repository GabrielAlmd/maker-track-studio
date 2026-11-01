#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
secrets_dir="$script_dir/secrets"
mkdir -p "$secrets_dir"
chmod 700 "$secrets_dir"

for name in proxy.key auth.htpasswd; do
    if [ -e "$secrets_dir/$name" ]; then
        printf 'Refusing to overwrite %s. Move it aside first if you intend to rotate credentials.\n' "$secrets_dir/$name" >&2
        exit 1
    fi
done

if ! command -v docker >/dev/null 2>&1; then
    printf 'Docker CLI is required to generate the password hash.\n' >&2
    exit 1
fi

printf 'Password for the shared Maker Track login: '
stty -echo
trap 'stty echo; printf "\n"' EXIT HUP INT TERM
IFS= read -r password
printf '\nConfirm password: '
IFS= read -r confirmation
stty echo
trap - EXIT HUP INT TERM
printf '\n'
if [ -z "$password" ] || [ "$password" != "$confirmation" ]; then
    printf 'Passwords were empty or did not match. No files were created.\n' >&2
    exit 1
fi

umask 077
tmpdir=$(mktemp -d "$secrets_dir/.setup.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM
key=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
printf '%s\n' "$key" > "$tmpdir/proxy.key"
# Password is sent over stdin, never included in Docker's command line.
hash=$(printf '%s\n' "$password" | docker run --rm -i --entrypoint selfhostctl "${MAKER_TRACK_IMAGE:?Set MAKER_TRACK_IMAGE to the selected published image}" hash-password)
unset password confirmation
[ -n "$hash" ] || { printf 'The helper did not return a password hash.\n' >&2; exit 1; }
printf '%s\n' "$hash" > "$tmpdir/auth.htpasswd"
chmod 600 "$tmpdir"/*
mv "$tmpdir/proxy.key" "$secrets_dir/proxy.key"
mv "$tmpdir/auth.htpasswd" "$secrets_dir/auth.htpasswd"
rmdir "$tmpdir"
trap - EXIT HUP INT TERM
printf 'Created protected proxy credentials in %s\n' "$secrets_dir"
# Preserve host ownership for password reset; grant only the container group access.
docker run --rm --user 0 --entrypoint sh -v "$secrets_dir:/secrets" "$MAKER_TRACK_IMAGE" -c 'chgrp -R 10001 /secrets; chmod 750 /secrets; chmod 640 /secrets/*'
printf 'Next: docker compose up -d --no-build\n'
