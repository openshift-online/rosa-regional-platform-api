#!/bin/bash
# CI entrypoint for unit tests.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

make test

if [[ -n "${ARTIFACT_DIR:-}" ]]; then
    cp -r coverage.* "${ARTIFACT_DIR}/" 2>/dev/null || true
fi
