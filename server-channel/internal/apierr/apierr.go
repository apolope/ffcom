// Package apierr escreve as respostas de erro da API HTTP no formato
// documentado em docs/protocol.md ("Erros da API HTTP"):
//
//	{"code":"friends.already_friends","message":"vocês já são amigos"}
//	{"code":"ideas.text_length","message":"...","params":{"min":10,"max":1000}}
//
// O code (`area.motivo`, snake_case) é estável e o front mostra
// t('errors.<code>', params) a partir de locales/<idioma>.json; a message
// fica em português, só como reserva (código sem tradução, client antigo) e
// para logs. Ver docs/architecture.md, "Decisão: internacionalização".
//
// O código é sempre uma string literal no terceiro argumento de Write e
// WriteParams (ou no primeiro de New e NewParams): o teste deste pacote e
// scripts/check-locales.mjs acham os códigos por regex e conferem que cada
// um tem chave errors.<code> em locales/pt-BR.json. Código montado em
// variável não é encontrado e o teste falha.
package apierr

import (
	"encoding/json"
	"net/http"
)

// Params são os valores interpolados na tradução ({{nome}} no i18next).
type Params map[string]any

// Problem é um erro ainda sem status, para funções de validação que só
// dizem o que está errado e deixam o handler responder com WriteProblem.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Params  Params `json:"params,omitempty"`
}

// New monta um Problem sem parâmetros.
func New(code, message string) *Problem {
	return &Problem{Code: code, Message: message}
}

// NewParams monta um Problem com parâmetros para a tradução.
func NewParams(code, message string, params Params) *Problem {
	return &Problem{Code: code, Message: message, Params: params}
}

// Write responde status com {"code":code,"message":message}.
func Write(w http.ResponseWriter, status int, code, message string) {
	WriteProblem(w, status, &Problem{Code: code, Message: message})
}

// WriteParams é Write com "params" para a interpolação no front.
func WriteParams(w http.ResponseWriter, status int, code, message string, params Params) {
	WriteProblem(w, status, &Problem{Code: code, Message: message, Params: params})
}

// WriteProblem responde status com p.
func WriteProblem(w http.ResponseWriter, status int, p *Problem) {
	h := w.Header()
	// Mesmo cuidado de http.Error: um Content-Length ou Content-Encoding
	// definido antes pelo handler não vale para este corpo.
	h.Del("Content-Length")
	h.Del("Content-Encoding")
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(p)
}
