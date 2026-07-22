#!/usr/bin/env bash
set -Eeuo pipefail

readonly test_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/opencloud-phase0-test-XXXXXX")"
trap 'rm -rf -- "${test_root}"' EXIT

export CADDYFILE_PATH="${test_root}/Caddyfile"
export CADDY_SNIPPET_DIR="${test_root}/conf.d"
# shellcheck source=run.sh
source "${test_script_dir}/run.sh"
sudo_cmd=()

assert_file_equals() {
  if ! cmp --silent "$1" "$2"; then
    echo "expected files to match: $1 $2" >&2
    exit 1
  fi
}

printf 'http://127.0.0.1 {\n    respond "production"\n}\n' >"${caddyfile}"
cp "${caddyfile}" "${test_root}/original"

ensure_caddy_import
ensure_caddy_import
test "$(grep -Fxc "import ${snippet_dir}/*.caddy" "${caddyfile}")" -eq 1
test -f "${caddy_backup}"
test -f "${caddy_owner_marker}"
restore_owned_caddyfile
assert_file_equals "${test_root}/original" "${caddyfile}"
rm -f "${caddy_backup}" "${caddy_owner_marker}"

printf 'http://127.0.0.1 {\n    respond "production"\n}\n\nimport %s/*.caddy\n' "${snippet_dir}" >"${caddyfile}"
cp "${caddyfile}" "${test_root}/preexisting-import"
ensure_caddy_import
test ! -e "${caddy_backup}"
test ! -e "${caddy_owner_marker}"
restore_owned_caddyfile
assert_file_equals "${test_root}/preexisting-import" "${caddyfile}"

cp "${test_root}/original" "${caddyfile}"
ensure_caddy_import
printf '# operator change\n' >>"${caddyfile}"
if restore_owned_caddyfile 2>/dev/null; then
  echo "expected checksum guard to reject a concurrent Caddyfile change" >&2
  exit 1
fi
grep -Fqx '# operator change' "${caddyfile}"

printf 'PASS caddy import ownership is idempotent, reversible, and conflict-safe\n'
