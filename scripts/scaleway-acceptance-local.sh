#!/usr/bin/env bash
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/k8s-acceptance-local.sh" scaleway "$@"
