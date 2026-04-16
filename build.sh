#!/usr/bin/env bash
set -euo pipefail

go build -trimpath -ldflags "-s -w" -o websearch ./go-websearch/main.go
printf 'Built ./websearch\n'
