#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
secrets_dir="$script_dir/secrets"
auth_file="$secrets_dir/auth.htpasswd"
[ -f "$auth_file" ] || { printf 'Missing %s; run setup-secrets.sh first.\n' "$auth_file" >&2; exit 1; }
[ -w "$secrets_dir" ] || { printf 'Cannot write to %s. Give your WSL user write access to the secrets directory.\n' "$secrets_dir" >&2; exit 1; }

printf 'New password for maker: '
stty -echo
trap 'stty echo; printf "\n"' EXIT HUP INT TERM
IFS= read -r password
printf '\nConfirm new password: '
IFS= read -r confirmation
stty echo
trap - EXIT HUP INT TERM
printf '\n'
if [ -z "$password" ] || [ "$password" != "$confirmation" ]; then
    printf 'Passwords were empty or did not match. No changes made.\n' >&2
    exit 1
fi
umask 077
tmp=$(mktemp "$secrets_dir/.auth.XXXXXX")
trap 'rm -f "$tmp"' EXIT HUP INT TERM
hash=$(printf '%s\n' "$password" | docker run --rm -i --entrypoint selfhostctl "${MAKER_TRACK_IMAGE:?Set MAKER_TRACK_IMAGE to the selected published image}" hash-password)
unset password confirmation
[ -n "$hash" ] || { printf 'The helper did not return a password hash.\n' >&2; exit 1; }
printf '%s\n' "$hash" > "$tmp"
chmod 640 "$tmp"
# Keep the container's existing owner/group so UID 10001 can read the file.
sudo chown --reference="$auth_file" "$tmp"
mv -f "$tmp" "$auth_file"
trap - EXIT HUP INT TERM
printf 'Password changed. Restarting the service to load it.\n'
cd "$script_dir"
docker compose up -d --force-recreate
