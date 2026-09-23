package httpapi

import (
	"context"
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

		resp, err := buildMeResponse(r.Context(), db.Profiles, account)
		if err != nil {
			http.Error(w, "erro ao buscar perfil", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

// buildMeResponse monta o meResponse da conta autenticada a partir do
// perfil (se já preenchido). Usado por handleMe e pelas rotas de avatar
// (internal/httpapi/avatar.go), que devolvem o mesmo formato depois de
// alterar o avatar.
func buildMeResponse(ctx context.Context, profiles *store.ProfileStore, account store.Account) (meResponse, error) {
	profile, err := profiles.GetByAccountID(ctx, account.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return meResponse{}, err
	}

	resp := meResponse{
		AccountID:    account.ID,
		OIDCSubject:  account.OIDCSubject,
		CreatedAt:    account.CreatedAt,
		E2EPublicKey: account.E2EPublicKey,
	}
	if err == nil {
		resp.DisplayName = &profile.DisplayName
		resp.AvatarURL = profile.AvatarURL
	}
	return resp, nil
}

type meResponse struct {
	AccountID   string    `json:"accountId"`
	OIDCSubject string    `json:"oidcSubject"`
	CreatedAt   time.Time `json:"createdAt"`
	// Sem omitempty de propósito: null (conta sem chave publicada) precisa
	// ser distinguível de campo ausente (server-central anterior a este
	// campo) -- o client só adota a chave de E2E antiga, de antes da chave
	// por conta, quando consegue comparar com esta (ver
	// client/src/hooks/useE2EKeys.ts).
	E2EPublicKey []byte  `json:"e2ePublicKey"`
	DisplayName  *string `json:"displayName,omitempty"`
	AvatarURL    *string `json:"avatarUrl,omitempty"`
}
