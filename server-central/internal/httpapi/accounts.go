package httpapi

import (
	"encoding/json"
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// maxLookupSubjects limita o tamanho de uma consulta: uma lista de membros
// de servidor grande cabe folgada, e ninguém usa a rota para varrer contas.
const maxLookupSubjects = 500

// POST /api/accounts/lookup — resolve oidc_subjects em contas (id, nome e
// avatar). server-channel guarda os membros pelo oidc_subject e não conversa
// com server-central (ver docs/architecture.md, "Decisão: protocolo/API"),
// então é o client que junta as duas pontas para mostrar o avatar de cada
// membro. Não devolve status de presença: esse continua só para amigos (ver
// "Decisão: status de presença e avatar nas listas de membros").
func handleLookupAccounts(accounts *store.AccountStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body lookupAccountsRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
			http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
			return
		}
		if len(body.Subjects) > maxLookupSubjects {
			http.Error(w, "subjects demais numa consulta", http.StatusBadRequest)
			return
		}

		found, err := accounts.GetManyBySubjects(r.Context(), body.Subjects)
		if err != nil {
			http.Error(w, "erro ao buscar contas", http.StatusInternalServerError)
			return
		}

		out := make([]accountSummaryView, len(found))
		for i, a := range found {
			out[i] = accountSummaryView{
				OIDCSubject: a.OIDCSubject,
				AccountID:   a.AccountID,
				DisplayName: a.DisplayName,
				AvatarURL:   a.AvatarURL,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lookupAccountsResponse{Accounts: out})
	})
}

type lookupAccountsRequest struct {
	Subjects []string `json:"subjects"`
}

type accountSummaryView struct {
	OIDCSubject string  `json:"oidcSubject"`
	AccountID   string  `json:"accountId"`
	DisplayName *string `json:"displayName,omitempty"`
	AvatarURL   *string `json:"avatarUrl,omitempty"`
}

type lookupAccountsResponse struct {
	Accounts []accountSummaryView `json:"accounts"`
}
