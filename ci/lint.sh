#!/bin/bash
# CI entrypoint for formatting and linting checks.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

make deps
make fmt
make lint
