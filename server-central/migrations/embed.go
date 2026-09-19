// Package migrations embute os arquivos .sql deste diretório no binário,
// para que o server-central não dependa de um diretório externo em disco
// para aplicar migrations no boot.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
