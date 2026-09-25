# Sobe o ambiente local de desenvolvimento do FFCom inteiro:
#   - server-central        http://localhost:8082
#   - server-channel "Teste" http://localhost:8080 (+ LiveKit e coturn)
#   - server-channel "Estúdio" http://localhost:8083 (docker-compose.dev-second.yml)
#   - client (Vite)         http://localhost:5173
# e aplica a estrutura de exemplo (categorias e canais) nos dois
# server-channel, sem duplicar nem desfazer a ordem já arrastada.
#
# Uso, na raiz do repositório:
#   ./scripts/dev-local.ps1            # sobe e deixa o Vite rodando
#   ./scripts/dev-local.ps1 -Build     # recompila as imagens antes
#   ./scripts/dev-local.ps1 -NoClient  # só os servidores
#
# Os dados ficam nos volumes do Docker, então servidores no rail, membros e
# ordem sobrevivem entre execuções. O script também põe toda conta que já
# logou localmente nos dois servidores (ver o passo de sincronizar, no fim).
# Ver CONTRIBUTING.md, "Ambiente local".

param(
    [switch]$Build,
    [switch]$NoClient
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

function Invoke-Compose {
    param([string]$Dir, [string[]]$ComposeArgs)
    Push-Location (Join-Path $root $Dir)
    try {
        $upArgs = @('compose') + $ComposeArgs + @('up', '-d')
        if ($Build) { $upArgs += '--build' }
        & docker @upArgs
        if ($LASTEXITCODE -ne 0) { throw "docker compose falhou em $Dir" }
    } finally {
        Pop-Location
    }
}

# Espera o app responder no /healthz: as migrations rodam no boot, e o seed
# precisa das tabelas.
function Wait-Healthy {
    param([string]$Url)
    for ($i = 0; $i -lt 60; $i++) {
        try {
            Invoke-WebRequest -Uri "$Url/healthz" -UseBasicParsing -TimeoutSec 2 | Out-Null
            return
        } catch {
            Start-Sleep -Seconds 1
        }
    }
    throw "$Url não respondeu em 60s"
}

function Invoke-Seed {
    param([string]$Container, [string]$SeedFile)
    Get-Content -Raw (Join-Path $root $SeedFile) | docker exec -i $Container psql -q -U ffcom -d ffcom_channel -v ON_ERROR_STOP=1
    if ($LASTEXITCODE -ne 0) { throw "seed $SeedFile falhou" }
}

Invoke-Compose 'server-central' @()
Invoke-Compose 'server-channel' @()
Invoke-Compose 'server-channel' @('-f', 'docker-compose.dev-second.yml')

Wait-Healthy 'http://localhost:8082'
Wait-Healthy 'http://localhost:8080'
Wait-Healthy 'http://localhost:8083'

Invoke-Seed 'server-channel-postgres-1' 'server-channel/dev/seed-teste.sql'
Invoke-Seed 'ffcom-channel-2-postgres-1' 'server-channel/dev/seed-estudio.sql'

# Toda conta que já logou no server-central local vira membro dos dois
# server-channel e ganha os dois no rail, sem convite: é o atalho que o
# fluxo real (entrar pelo "+" com convite) não dá para contas de teste. Conta
# nova só existe depois do primeiro login local, então quem logar pela
# primeira vez precisa rodar o script de novo. Membro expulso continua
# expulso (ON CONFLICT DO NOTHING). Servidor sem dono ganha como dono a
# conta mais antiga, que é a de quem montou o ambiente.
$subjects = @(docker exec server-central-postgres-1 psql -At -U ffcom -d ffcom_central -c 'SELECT oidc_subject FROM accounts ORDER BY created_at')
$subjects = @($subjects | Where-Object { $_ -match '^[A-Za-z0-9._@-]+$' })
if ($subjects.Count -gt 0) {
    $values = ($subjects | ForEach-Object { "('$_')" }) -join ', '
    $membersSql = @"
INSERT INTO members (oidc_subject) VALUES $values ON CONFLICT (oidc_subject) DO NOTHING;
UPDATE members SET is_owner = true
WHERE oidc_subject = '$($subjects[0])' AND NOT EXISTS (SELECT 1 FROM members WHERE is_owner);
"@
    foreach ($container in 'server-channel-postgres-1', 'ffcom-channel-2-postgres-1') {
        $membersSql | docker exec -i $container psql -q -U ffcom -d ffcom_channel -v ON_ERROR_STOP=1
        if ($LASTEXITCODE -ne 0) { throw "sincronizar membros em $container falhou" }
    }
    @"
INSERT INTO known_servers (account_id, address, name, position)
SELECT a.id, v.address, v.name,
       (SELECT COALESCE(MAX(k.position) + 1, 0) FROM known_servers k WHERE k.account_id = a.id) + v.ord
FROM accounts a
CROSS JOIN (VALUES ('http://localhost:8080', 'Teste', 0), ('http://localhost:8083', 'Estúdio', 1)) AS v(address, name, ord)
ON CONFLICT (account_id, address) DO NOTHING;
"@ | docker exec -i server-central-postgres-1 psql -q -U ffcom -d ffcom_central -v ON_ERROR_STOP=1
    if ($LASTEXITCODE -ne 0) { throw 'adicionar servidores ao rail falhou' }
}

Write-Host ''
Write-Host 'Servidores no ar: central :8082, Teste :8080, Estúdio :8083.'
Write-Host "Contas locais com os dois servidores no rail: $($subjects.Count). Conta que logar pela primeira vez: rode o script de novo."

if (-not $NoClient) {
    Push-Location (Join-Path $root 'client')
    try {
        npm run dev -- --port 5173 --strictPort
    } finally {
        Pop-Location
    }
}
