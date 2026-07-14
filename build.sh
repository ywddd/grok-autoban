#!/usr/bin/env bash
set -euo pipefail

export CGO_ENABLED=1
: "${CC:=cc}"
go build -buildmode=c-shared -o grok-autoban.so .
echo "已生成 $(pwd)/grok-autoban.so"
