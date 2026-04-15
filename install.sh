#!/usr/bin/env bash
set -euo pipefail

REPO="forechoandlook/gtask"
INSTALL_DIR="${HOME}/.local/bin"
BIN_NAME="gtask"

uname_s="$(uname -s)"
uname_m="$(uname -m)"

case "${uname_s}" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *)
    echo "unsupported OS: ${uname_s}" >&2
    exit 1
    ;;
esac

case "${uname_m}" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported architecture: ${uname_m}" >&2
    exit 1
    ;;
esac

url="https://github.com/${REPO}/releases/latest/download/${BIN_NAME}_${os}_${arch}"
tmp="$(mktemp)"
mkdir -p "${INSTALL_DIR}"

echo "downloading ${url}"
curl -fsSL "${url}" -o "${tmp}"
chmod +x "${tmp}"
mv "${tmp}" "${INSTALL_DIR}/${BIN_NAME}"

echo "installed to ${INSTALL_DIR}/${BIN_NAME}"
echo "run: ${BIN_NAME} --version"
