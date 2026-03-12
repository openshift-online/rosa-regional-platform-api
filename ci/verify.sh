#!/bin/bash
# CI entrypoint for go.mod/go.sum tidiness verification.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

make verify
