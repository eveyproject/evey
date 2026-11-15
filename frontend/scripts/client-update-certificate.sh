#!/usr/bin/env bash
# Certificates behind the app updates an instance serves (docs/client-updates.md).
#
#   client-update-certificate.sh root <dir>          once: the root every app build carries
#   client-update-certificate.sh leaf <dir> [days]   yearly: the certificate releases are signed with
#
# root writes <dir>/root-key.pem, encrypted under a passphrase openssl asks
# for, and client-updates/certificate.pem, which is committed. Keep the root
# key offline and backed up: it never goes to a build machine.
# leaf writes <dir>/leaf-key.pem and <dir>/leaf-cert.pem, signed by the root
# key in <dir>. Those two are what a release build needs.
#
# EVEY_CLIENT_UPDATE_ROOT_PASS_FILE names a file holding the passphrase, for a
# run without a terminal.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
committed="${here}/../client-updates/certificate.pem"
mode="${1:-}"
dir="${2:-}"
[ -n "${dir}" ] || { echo "usage: $0 root|leaf <dir> [days]" >&2; exit 2; }
mkdir -p "${dir}"
chmod 700 "${dir}"
umask 077

pass=()
[ -z "${EVEY_CLIENT_UPDATE_ROOT_PASS_FILE:-}" ] || pass=("file:${EVEY_CLIENT_UPDATE_ROOT_PASS_FILE}")

case "${mode}" in
root)
  [ ! -e "${dir}/root-key.pem" ] || { echo "${dir}/root-key.pem exists: a new root means a new store build of both apps, remove it yourself if that is the intent" >&2; exit 1; }
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 -aes-256-cbc ${pass[0]:+-pass "${pass[0]}"} -out "${dir}/root-key.pem"
  openssl req -x509 -new -key "${dir}/root-key.pem" ${pass[0]:+-passin "${pass[0]}"} -sha256 -days 10950 \
    -subj "/CN=Evey client updates root" \
    -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
    -addext "keyUsage=critical,keyCertSign,cRLSign" \
    -out "${committed}"
  echo "root certificate written to ${committed}; root key in ${dir}/root-key.pem"
  ;;
leaf)
  days="${3:-365}"
  work="$(mktemp -d)"
  trap 'rm -rf "${work}"' EXIT
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "${dir}/leaf-key.pem"
  openssl req -new -key "${dir}/leaf-key.pem" -subj "/CN=Evey client updates signing" -out "${work}/leaf.csr"
  printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=critical,codeSigning\n' > "${work}/leaf.ext"
  openssl x509 -req -in "${work}/leaf.csr" -CA "${committed}" -CAkey "${dir}/root-key.pem" ${pass[0]:+-passin "${pass[0]}"} \
    -CAcreateserial -CAserial "${work}/serial" -sha256 -days "${days}" -extfile "${work}/leaf.ext" -out "${dir}/leaf-cert.pem"
  openssl verify -CAfile "${committed}" "${dir}/leaf-cert.pem" >/dev/null
  echo "signing certificate written to ${dir}/leaf-cert.pem, valid until $(openssl x509 -in "${dir}/leaf-cert.pem" -noout -enddate | cut -d= -f2)"
  ;;
*)
  echo "usage: $0 root|leaf <dir> [days]" >&2
  exit 2
  ;;
esac
