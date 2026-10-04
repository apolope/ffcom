package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// GET /api/me — devolve a conta autenticada (criada no primeiro login pelo
// próprio auth.Middleware) e o perfil, se já tiver sido preenchido.
func handleMe(db *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		resp, err := buildMeResponse(r.Context(), db.Profiles, account)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "profile.fetch_failed", "erro ao buscar perfil")
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
		AccountID:       account.ID,
		OIDCSubject:     account.OIDCSubject,
		CreatedAt:       account.CreatedAt,
		E2EPublicKey:    account.E2EPublicKey,
		HasE2EKeyBackup: account.HasE2EKeyBackup,
		Status:          account.PresenceStatus,
		Language:        account.Language,
	}
	if err == nil {
		resp.DisplayName = &profile.DisplayName
		resp.AvatarURL = profile.AvatarURL
	}

	stored, err := profiles.StoredDisplayName(ctx, account.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return meResponse{}, err
	}
	if err == nil && stored != account.OIDCSubject {
		resp.CustomDisplayName = &stored
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
	E2EPublicKey []byte `json:"e2ePublicKey"`
	// Se a conta já tem o backup cifrado da chave privada (GET
	// /api/me/e2e-key-backup): o client decide entre criar a frase de
	// recuperação ou pedir a existente sem baixar o backup à toa.
	HasE2EKeyBackup bool `json:"hasE2EKeyBackup"`
	// Status escolhido (online, busy, away ou invisible), não o visível.
	Status      string  `json:"status"`
	DisplayName *string `json:"displayName,omitempty"`
	// Nome escolhido no FFCom (PUT /api/me/display-name), ausente quando
	// displayName é o do Authentik. O client preenche o diálogo com ele:
	// com displayName, salvar sem mexer congelaria o nome do Authentik.
	CustomDisplayName *string `json:"customDisplayName,omitempty"`
	AvatarURL         *string `json:"avatarUrl,omitempty"`
	// Idioma da interface escolhido (PATCH /api/me). Sem omitempty: null
	// (nunca escolheu, o client segue o localStorage e o navegador) é
	// diferente de campo ausente (server-central anterior a este campo).
	Language *string `json:"language"`
}

// PATCH /api/me: altera preferências da conta autenticada. Só os campos
// presentes no corpo mudam; hoje, só "language" (um de
// store.SupportedLanguages, ou null para apagar a escolha). Devolve o mesmo
// formato de GET /api/me. Ver docs/architecture.md, "Decisão:
// internacionalização".
func handlePatchMe(db *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}

		if raw, present := body["language"]; present {
			var language *string
			if err := json.Unmarshal(raw, &language); err != nil {
				apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
				return
			}
			if language != nil && !store.ValidLanguage(*language) {
				apierr.WriteParams(w, http.StatusBadRequest, "profile.language_unsupported", "idioma não suportado: use pt-BR ou en",
					apierr.Params{"supported": strings.Join(store.SupportedLanguages, ", ")})
				return
			}
			if err := db.Accounts.SetLanguage(r.Context(), account.ID, language); err != nil {
				log.Printf("server-central: erro ao salvar idioma: %v", err)
				apierr.Write(w, http.StatusInternalServerError, "profile.language_save_failed", "erro ao salvar idioma")
				return
			}
			account.Language = language
		}

		respondMe(w, r, db.Profiles, account)
	})
}
