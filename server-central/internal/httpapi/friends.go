package httpapi

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// inviteCodeAlphabet é o alfabeto do base32 padrão (A-Z2-7) sem padding —
// evita os caracteres visualmente ambíguos 0/O e 1/I que um alfabeto
// hexadecimal ou base64 teria, já que o código é para copiar/colar ou
// digitar manualmente.
const inviteCodeLength = 10

// generateInviteCode gera um código aleatório de inviteCodeLength
// caracteres (base32, sem padding) para convites de amizade.
func generateInviteCode() (string, error) {
	buf := make([]byte, inviteCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return encoded[:inviteCodeLength], nil
}

// POST /api/friends/invites — gera um novo código de convite de amizade
// para a conta autenticada compartilhar por fora (ver docs/architecture.md,
// "Decisão: adicionar amigos via convite"). Sem expiração: o código só
// deixa de valer depois de resgatado uma vez.
func handleCreateFriendInvite(invites *store.FriendInviteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		var invite store.FriendInvite
		// Colisão de código é astronomicamente improvável (10 chars base32 =
		// 50 bits de entropia), mas o retry mantém o endpoint correto mesmo
		// assim em vez de assumir unicidade.
		for attempt := 0; attempt < 5; attempt++ {
			code, err := generateInviteCode()
			if err != nil {
				http.Error(w, "erro ao gerar código de convite", http.StatusInternalServerError)
				return
			}
			invite, err = invites.Create(r.Context(), code, account.ID)
			if err == nil {
				break
			}
			if attempt == 4 {
				http.Error(w, "erro ao criar convite", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(friendInviteView{Code: invite.Code, CreatedAt: invite.CreatedAt})
	})
}

// POST /api/friends/invites/{code}/redeem — resgata um convite de amizade,
// criando a amizade já como "accepted" entre quem criou o código e quem
// resgatou (resgatar é o consentimento mútuo, ver docs/architecture.md).
func handleRedeemFriendInvite(invites *store.FriendInviteStore, friendships *store.FriendshipStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		code := r.PathValue("code")
		invite, err := invites.GetByCode(r.Context(), code)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "convite não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao buscar convite", http.StatusInternalServerError)
			return
		}
		if invite.CreatedByAccountID == account.ID {
			http.Error(w, "não é possível resgatar seu próprio convite", http.StatusBadRequest)
			return
		}
		if invite.RedeemedAt != nil {
			http.Error(w, "convite já foi usado", http.StatusConflict)
			return
		}
		if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now()) {
			http.Error(w, "convite expirado", http.StatusGone)
			return
		}

		if _, err := invites.Redeem(r.Context(), invite.ID, account.ID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				http.Error(w, "convite já foi usado", http.StatusConflict)
				return
			}
			http.Error(w, "erro ao resgatar convite", http.StatusInternalServerError)
			return
		}

		if _, err := friendships.CreateAccepted(r.Context(), invite.CreatedByAccountID, account.ID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				http.Error(w, "vocês já são amigos", http.StatusConflict)
				return
			}
			http.Error(w, "erro ao criar amizade", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(redeemInviteResponse{FriendAccountID: invite.CreatedByAccountID})
	})
}

// GET /api/friends — lista as amizades aceitas da conta autenticada, com o
// perfil (nome de exibição, avatar) e a chave pública de E2E (se já
// publicada, ver docs/architecture.md, "Decisão: criptografia ponta-a-ponta
// em DMs") de cada amigo quando disponíveis. Não inclui status
// online/offline — isso vem de GET /api/presence, que o client combina com
// esta lista (ver internal/httpapi/presence.go).
func handleListFriends(friendships *store.FriendshipStore, profiles *store.ProfileStore, accounts *store.AccountStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		friendIDs, err := friendships.AcceptedFriendIDs(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "erro ao buscar amigos", http.StatusInternalServerError)
			return
		}

		profileByAccount, err := profiles.GetManyByAccountIDs(r.Context(), friendIDs)
		if err != nil {
			http.Error(w, "erro ao buscar perfis", http.StatusInternalServerError)
			return
		}

		accountByID, err := accounts.GetManyByIDs(r.Context(), friendIDs)
		if err != nil {
			http.Error(w, "erro ao buscar contas", http.StatusInternalServerError)
			return
		}

		out := make([]friendView, len(friendIDs))
		for i, friendID := range friendIDs {
			view := friendView{AccountID: friendID}
			if profile, ok := profileByAccount[friendID]; ok {
				view.DisplayName = &profile.DisplayName
				view.AvatarURL = profile.AvatarURL
			}
			if acc, ok := accountByID[friendID]; ok {
				view.E2EPublicKey = acc.E2EPublicKey
			}
			out[i] = view
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listFriendsResponse{Friends: out})
	})
}

type friendInviteView struct {
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"createdAt"`
}

type redeemInviteResponse struct {
	FriendAccountID string `json:"friendAccountId"`
}

type friendView struct {
	AccountID    string  `json:"accountId"`
	DisplayName  *string `json:"displayName,omitempty"`
	AvatarURL    *string `json:"avatarUrl,omitempty"`
	E2EPublicKey []byte  `json:"e2ePublicKey,omitempty"`
}

type listFriendsResponse struct {
	Friends []friendView `json:"friends"`
}
