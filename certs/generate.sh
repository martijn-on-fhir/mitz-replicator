#!/usr/bin/env bash
set -euo pipefail

# Prevent MSYS/Git Bash from converting /CN= to a Windows path
export MSYS_NO_PATHCONV=1

cd "$(dirname "$0")"

echo "==> Generating CA key and certificate..."
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout ca.key -out ca.crt -days 365 \
  -subj "/CN=Mitz Test CA"

echo "==> Generating server key and certificate..."
openssl req -newkey rsa:2048 -nodes \
  -keyout server.key -out server.csr \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

openssl x509 -req -in server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 365 \
  -copy_extensions copyall

echo "==> Generating client key and certificate..."
openssl req -newkey rsa:2048 -nodes \
  -keyout client.key -out client.csr \
  -subj "/CN=mitz-connector"

openssl x509 -req -in client.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt -days 365

echo "==> Cleaning up CSR files..."
rm -f *.csr *.srl

echo "==> Done! Generated files:"
ls -la *.crt *.key
