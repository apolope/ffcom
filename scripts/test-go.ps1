# Roda go vet + go test de server-central e server-channel contra um Postgres
# descartável, do mesmo jeito que o ci.yml: sobe um postgres:17-alpine só para
# esta execução (dados em tmpfs, porta aleatória em 127.0.0.1), aponta
# FFCOM_TEST_DATABASE_URL para ele e remove o container no fim, mesmo se os
# testes falharem ou a execução for interrompida com Ctrl+C.
#
# Uso, na raiz do repositório:
#   ./scripts/test-go.ps1                      # os dois módulos
#   ./scripts/test-go.ps1 -Module channel      # só server-channel
#   ./scripts/test-go.ps1 -Run EndToEnd        # repassa -run ao go test
#
# Cada módulo usa um banco próprio no mesmo container, porque as migrations
# dos dois são diferentes. Ver CONTRIBUTING.md, "Testes e lint".

param(
    [ValidateSet('all', 'central', 'channel')]
    [string]$Module = 'all',
    [string]$Run
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

# O instalador do Go no Windows nem sempre põe o go no PATH do PowerShell.
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $goBin = 'C:\Program Files\Go\bin'
    if (-not (Test-Path (Join-Path $goBin 'go.exe'))) { throw 'go não encontrado no PATH nem em C:\Program Files\Go\bin' }
    $env:PATH = "$goBin;$env:PATH"
}

$modules = if ($Module -eq 'all') { @('central', 'channel') } else { @($Module) }
$container = "ffcom-test-pg-$PID"

& docker run -d --rm --name $container `
    -e POSTGRES_PASSWORD=test `
    --tmpfs /var/lib/postgresql/data `
    -p 127.0.0.1::5432 `
    postgres:17-alpine | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'docker run do Postgres falhou (o Docker está rodando?)' }

$failed = @()
try {
    $deadline = (Get-Date).AddSeconds(60)
    while ($true) {
        & docker exec $container pg_isready -U postgres -q 2>$null
        if ($LASTEXITCODE -eq 0) { break }
        if ((Get-Date) -gt $deadline) { throw 'Postgres não ficou pronto em 60s' }
        Start-Sleep -Milliseconds 500
    }
    $port = ((& docker port $container 5432/tcp) | Select-Object -First 1).Split(':')[-1]

    foreach ($m in $modules) {
        $db = "ffcom_${m}_test"
        # pg_isready responde durante o boot do initdb; o CREATE DATABASE só vale
        # quando o servidor definitivo já aceita conexão, então tenta de novo.
        $deadline = (Get-Date).AddSeconds(30)
        while ($true) {
            & docker exec $container psql -U postgres -q -c "CREATE DATABASE $db" 2>$null
            if ($LASTEXITCODE -eq 0) { break }
            if ((Get-Date) -gt $deadline) { throw "não consegui criar o banco $db" }
            Start-Sleep -Milliseconds 500
        }

        $env:FFCOM_TEST_DATABASE_URL = "postgres://postgres:test@127.0.0.1:$port/${db}?sslmode=disable"
        Push-Location (Join-Path $root "server-$m")
        try {
            Write-Host "== server-${m}: go vet" -ForegroundColor Cyan
            & go vet ./...
            if ($LASTEXITCODE -ne 0) { $failed += "server-$m (vet)"; continue }

            Write-Host "== server-${m}: go test" -ForegroundColor Cyan
            $testArgs = @('test', './...')
            if ($Run) { $testArgs += @('-run', $Run) }
            & go @testArgs
            if ($LASTEXITCODE -ne 0) { $failed += "server-$m (test)" }
        } finally {
            Pop-Location
        }
    }
} finally {
    Remove-Item Env:FFCOM_TEST_DATABASE_URL -ErrorAction SilentlyContinue
    & docker stop $container | Out-Null
}

if ($failed.Count -gt 0) {
    Write-Host "Falhou: $($failed -join ', ')" -ForegroundColor Red
    exit 1
}
Write-Host 'Tudo passou.' -ForegroundColor Green
