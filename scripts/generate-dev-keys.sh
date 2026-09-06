#!/bin/sh
set -eu

key_dir=/run/secrets/jwt
private_key="$key_dir/private.pem"
public_key="$key_dir/public.pem"

mkdir -p "$key_dir"
umask 077

if [ -s "$private_key" ]; then
    openssl pkey -in "$private_key" -check -noout >/dev/null
    public_tmp=$(mktemp "$key_dir/public.pem.XXXXXX")
    trap 'rm -f "$public_tmp"' EXIT
    openssl pkey -in "$private_key" -pubout -out "$public_tmp"
    chmod 0644 "$public_tmp"
    mv "$public_tmp" "$public_key"
    chown 10001:10001 "$private_key" "$public_key"
    trap - EXIT
    echo "Existing development JWT keypair is valid."
    exit 0
fi

private_tmp=$(mktemp "$key_dir/private.pem.XXXXXX")
public_tmp=$(mktemp "$key_dir/public.pem.XXXXXX")
trap 'rm -f "$private_tmp" "$public_tmp"' EXIT

openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "$private_tmp" 2>/dev/null
openssl pkey -in "$private_tmp" -pubout -out "$public_tmp"
chmod 0600 "$private_tmp"
chmod 0644 "$public_tmp"
mv "$private_tmp" "$private_key"
mv "$public_tmp" "$public_key"
chown 10001:10001 "$private_key" "$public_key"
trap - EXIT

echo "Generated an ephemeral development JWT keypair in the Docker volume."
