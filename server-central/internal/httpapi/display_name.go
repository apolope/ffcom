package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"unicode"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// maxDisplayNameBytes é o mesmo limite do apelido e do profile_name em
// server-channel: o client grava este nome lá como nome do perfil, então
// ele precisa caber.
const maxDisplayNameBytes = 64

// PUT /api/me/display-name — grava o nome de exibição escolhido no FFCom,
// que vale em todo lugar (amigos, DMs, e cada server-channel onde a pessoa
// não tem apelido). displayName vazio ou null volta a seguir o nome do
// perfil do Authentik. Devolve o mesmo formato de GET /api/me. Ver
// docs/architecture.md, "Decisão: nome de exibição da conta".
func handleSetDisplayName(profiles *store.ProfileStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		var body struct {
			DisplayName *string `json:"displayName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
			return
		}

		// O "sub" é o marcador de "sem nome escolhido" (ver displayNameSQL),
		// o mesmo que as rotas de avatar gravam.
		name := account.OIDCSubject
		if body.DisplayName != nil {
			trimmed := strings.TrimSpace(*body.DisplayName)
			if len(trimmed) > maxDisplayNameBytes {
				http.Error(w, "nome deve ter no máximo 64 caracteres", http.StatusBadRequest)
				return
			}
			if strings.ContainsFunc(trimmed, unicode.IsControl) {
				http.Error(w, "nome não pode ter quebra de linha nem caractere de controle", http.StatusBadRequest)
				return
			}
			if trimmed != "" {
				name = trimmed
			}
		}

		if err := profiles.SetDisplayName(r.Context(), account.ID, name); err != nil {
			log.Printf("server-central: erro ao salvar nome de exibição: %v", err)
			http.Error(w, "erro ao salvar nome", http.StatusInternalServerError)
			return
		}

		respondMe(w, r, profiles, account)
	})
}
