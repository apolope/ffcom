package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// GET /api/me — devolve a conta autenticada (criada no primeiro login pelo
// próprio auth.Middleware) e o perfil, se já tiver sido preenchido.
func handleMe(db *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		profile, err := db.Profiles.GetByAccountID(r.Context(), account.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			http.Error(w, "erro ao buscar perfil", http.StatusInternalServerError)
			return
		}

		resp := meResponse{
			AccountID:   account.ID,
			OIDCSubject: account.OIDCSubject,
			CreatedAt:   account.CreatedAt,
		}
		if err == nil {
			resp.DisplayName = &profile.DisplayName
			resp.AvatarURL = profile.AvatarURL
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

type meResponse struct {
	AccountID   string    `json:"accountId"`
	OIDCSubject string    `json:"oidcSubject"`
	CreatedAt   time.Time `json:"createdAt"`
	DisplayName *string   `json:"displayName,omitempty"`
	AvatarURL   *string   `json:"avatarUrl,omitempty"`
}
