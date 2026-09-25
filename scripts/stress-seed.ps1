# Popula o ambiente local com dados em massa para teste de stress da
# interface: centenas de membros, dezenas de categorias e canais, um canal de
# texto (stress-chat) e um fórum (stress-forum) cheios, amigos e servidores
# extras no rail. Precisa do ambiente de scripts/dev-local.ps1 no ar.
#
# Uso, na raiz do repositório:
#   ./scripts/stress-seed.ps1                 # recria os dados de stress
#   ./scripts/stress-seed.ps1 -Members 1000   # mais volume
#   ./scripts/stress-seed.ps1 -Remove         # apaga só os dados de stress
#
# Cada execução apaga os dados de stress anteriores antes de criar de novo;
# nada fora deles é tocado (membros 'stress-NNNN', categorias e roles
# 'Stress ...', servidores 'http://stress-NN.invalid'). Os SQL ficam em
# scripts/stress/. Recarregue o client depois de rodar.

param(
    [switch]$Remove,
    [int]$Members = 300,
    [int]$Categories = 15,
    [int]$ChannelsPerCategory = 10,
    [int]$ChatMessages = 1500,
    [int]$ForumThreads = 80,
    [int]$ThreadMessages = 30,
    [int]$Friends = 150,
    [int]$RailServers = 25
)

$ErrorActionPreference = 'Stop'
$stress = Join-Path $PSScriptRoot 'stress'

function Invoke-Sql {
    param([string]$Container, [string]$Database, [string]$File, [hashtable]$Vars = @{})
    $psqlArgs = @('exec', '-i', $Container, 'psql', '-q', '-U', 'ffcom', '-d', $Database, '-v', 'ON_ERROR_STOP=1')
    foreach ($name in $Vars.Keys) { $psqlArgs += @('-v', "$name=$($Vars[$name])") }
    Get-Content -Raw -Encoding UTF8 (Join-Path $stress $File) | docker @psqlArgs
    if ($LASTEXITCODE -ne 0) { throw "$File falhou em $Container" }
}

$channelContainers = 'server-channel-postgres-1', 'ffcom-channel-2-postgres-1'

foreach ($container in $channelContainers) {
    Invoke-Sql $container 'ffcom_channel' 'channel-remove.sql'
}
Invoke-Sql 'server-central-postgres-1' 'ffcom_central' 'central-remove.sql'

if ($Remove) {
    Write-Host 'Dados de stress removidos.'
    return
}

$channelVars = @{
    members               = $Members
    categories            = $Categories
    channels_per_category = $ChannelsPerCategory
    chat_messages         = $ChatMessages
    forum_threads         = $ForumThreads
    thread_messages       = $ThreadMessages
}
foreach ($container in $channelContainers) {
    Invoke-Sql $container 'ffcom_channel' 'channel-seed.sql' $channelVars
}
Invoke-Sql 'server-central-postgres-1' 'ffcom_central' 'central-seed.sql' @{
    members      = $Members
    friends      = [Math]::Min($Friends, $Members)
    rail_servers = $RailServers
}

Write-Host ''
Write-Host "Stress nos dois server-channel: $Members membros, $Categories categorias x $ChannelsPerCategory canais,"
Write-Host "  stress-chat com $ChatMessages mensagens, stress-forum com $ForumThreads threads x $ThreadMessages mensagens."
Write-Host "Central: $([Math]::Min($Friends, $Members)) amigos e $RailServers servidores extras no rail por conta real."
Write-Host 'Recarregue o client. Para desfazer: ./scripts/stress-seed.ps1 -Remove'
