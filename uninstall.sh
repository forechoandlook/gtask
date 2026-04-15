#!/usr/bin/env bash
set -euo pipefail

BIN_PATH="${HOME}/.local/bin/gtask"

if [[ -f "${BIN_PATH}" ]]; then
  rm -f "${BIN_PATH}"
  echo "removed ${BIN_PATH}"
else
  echo "not installed: ${BIN_PATH}"
fi
