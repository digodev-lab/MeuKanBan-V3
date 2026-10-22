# Compila o aplicativo nativo no Windows com Go instalado.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
New-Item -ItemType Directory -Force -Path 'build' | Out-Null
Write-Host 'Executando testes...'
go test ./...
if ($LASTEXITCODE -ne 0) { throw 'Os testes falharam.' }
Write-Host 'Compilando Windows 64 bits...'
go build -ldflags '-H windowsgui -s -w' -o 'build/MeuKanban_Pro_V3.exe' .
if ($LASTEXITCODE -ne 0) { throw 'A compilacao falhou.' }
Write-Host 'Arquivo criado: build/MeuKanban_Pro_V3.exe'
