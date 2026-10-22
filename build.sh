#!/usr/bin/env bash
# Cross-compila o projeto Windows em Linux/macOS usando Go.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p build
printf '%s\n' 'Executando testes nativos dos modulos portaveis...'
go test ./...
printf '%s\n' 'Compilando para Windows 64 bits...'
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags='-H windowsgui -s -w' -o build/MeuKanban_Pro_V3.exe .
printf '%s\n' 'Arquivo criado: build/MeuKanban_Pro_V3.exe'
