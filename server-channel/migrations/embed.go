// Package migrations embute os arquivos .sql deste diretório no binário,
// para que o server-channel não dependa de um diretório externo em disco
// para aplicar migrations no boot.
//
// Disciplina expand/contract: uma migration nunca quebra o binário da
// versão anterior, porque o ffcom-runtime pode voltar para ela depois da
// migration aplicada (o boot tolera schema à frente, ver
// internal/store.migrateUp). Coluna/tabela nova entra opcional ou com
// default; renomear ou remover só numa versão seguinte, quando nenhum
// binário ainda suportado usa mais o nome antigo. Ver docs/architecture.md,
// "Decisão: container evergreen em server-channel".
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
