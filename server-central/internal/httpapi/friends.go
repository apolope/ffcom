package httpapi

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/push"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
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
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		var invite store.FriendInvite
		// Colisão de código é astronomicamente improvável (10 chars base32 =
		// 50 bits de entropia), mas o retry mantém o endpoint correto mesmo
		// assim em vez de assumir unicidade.
		for attempt := 0; attempt < 5; attempt++ {
			code, err := generateInviteCode()
			if err != nil {
				apierr.Write(w, http.StatusInternalServerError, "friends.invite_code_failed", "erro ao gerar código de convite")
				return
			}
			invite, err = invites.Create(r.Context(), code, account.ID)
			if err == nil {
				break
			}
			if attempt == 4 {
				apierr.Write(w, http.StatusInternalServerError, "friends.invite_create_failed", "erro ao criar convite")
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
func handleRedeemFriendInvite(hub *realtime.Hub, invites *store.FriendInviteStore, friendships *store.FriendshipStore, profiles *store.ProfileStore, dispatcher *push.Dispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		code := r.PathValue("code")
		invite, err := invites.GetByCode(r.Context(), code)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "friends.invite_not_found", "convite não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "friends.invite_fetch_failed", "erro ao buscar convite")
			return
		}
		if invite.CreatedByAccountID == account.ID {
			apierr.Write(w, http.StatusBadRequest, "friends.invite_own", "não é possível resgatar seu próprio convite")
			return
		}
		if invite.RedeemedAt != nil {
			apierr.Write(w, http.StatusConflict, "friends.invite_used", "convite já foi usado")
			return
		}
		if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now()) {
			apierr.Write(w, http.StatusGone, "friends.invite_expired", "convite expirado")
			return
		}

		if _, err := invites.Redeem(r.Context(), invite.ID, account.ID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				apierr.Write(w, http.StatusConflict, "friends.invite_used", "convite já foi usado")
				return
			}
			apierr.Write(w, http.StatusInternalServerError, "friends.invite_redeem_failed", "erro ao resgatar convite")
			return
		}

		friendship, err := friendships.CreateAccepted(r.Context(), invite.CreatedByAccountID, account.ID)
		if errors.Is(err, store.ErrConflict) {
			// Um pedido pendente entre os dois (ver friend_requests.go) não
			// impede o convite: resgatar já é o consentimento dos dois lados.
			existing, betweenErr := friendships.Between(r.Context(), invite.CreatedByAccountID, account.ID)
			if betweenErr != nil || existing.Status != store.FriendshipPending {
				apierr.Write(w, http.StatusConflict, "friends.already_friends", "vocês já são amigos")
				return
			}
			friendship, err = friendships.SetStatus(r.Context(), existing.ID, store.FriendshipAccepted)
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "friends.create_failed", "erro ao criar amizade")
			return
		}
		notifyFriendAccepted(r.Context(), hub, profiles, friendship)
		// Quem criou o convite fica sabendo pelo celular; quem resgatou está
		// com o app aberto.
		pushFromAccount(dispatcher, profiles, push.TypeFriendAccepted, invite.CreatedByAccountID, account.ID, "")

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
// esta lista (ver internal/httpapi/presence.go). lastMessageAt (ver
// DirectMessageStore.LastMessageAtByPeer) alimenta o indicador de não lida no
// client, mesmo mecanismo do lastMessageAt de canal em server-channel — ver
// docs/architecture.md, "Decisão: indicador de não lida".
func handleListFriends(friendships *store.FriendshipStore, profiles *store.ProfileStore, accounts *store.AccountStore, directMessages *store.DirectMessageStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		friendIDs, err := friendships.AcceptedFriendIDs(r.Context(), account.ID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "friends.fetch_failed", "erro ao buscar amigos")
			return
		}

		profileByAccount, err := profiles.GetManyByAccountIDs(r.Context(), friendIDs)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "profile.fetch_many_failed", "erro ao buscar perfis")
			return
		}

		accountByID, err := accounts.GetManyByIDs(r.Context(), friendIDs)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "accounts.fetch_failed", "erro ao buscar contas")
			return
		}

		lastMessageAt, err := directMessages.LastMessageAtByPeer(r.Context(), account.ID, friendIDs)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "friends.activity_fetch_failed", "erro ao buscar atividade das conversas")
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
			if at, ok := lastMessageAt[friendID]; ok {
				view.LastMessageAt = &at
			}
			out[i] = view
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listFriendsResponse{Friends: out})
	})
}

// DELETE /api/friends/{accountId} — desfaz a amizade aceita com accountId
// (qualquer um dos dois lados pode). Os dois recebem friend.removed pelo
// WebSocket de presença, para a lista atualizar em todas as abas; a partir
// daí presença e DMs param de fluir entre eles, porque ambos dependem de
// AcceptedFriendIDs/AreFriends a cada evento.
func handleDeleteFriend(hub *realtime.Hub, friendships *store.FriendshipStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		friendID := r.PathValue("accountId")
		if _, err := friendships.DeleteAccepted(r.Context(), account.ID, friendID); err != nil {
			// Id que não é UUID também cai aqui; para quem chamou, é igual a
			// não serem amigos.
			apierr.Write(w, http.StatusNotFound, "friends.not_friends", "vocês não são amigos")
			return
		}

		if payload, err := realtime.EncodeFriendRemoved(friendID); err == nil {
			hub.SendTo(account.ID, payload)
		}
		if payload, err := realtime.EncodeFriendRemoved(account.ID); err == nil {
			hub.SendTo(friendID, payload)
		}
		w.WriteHeader(http.StatusNoContent)
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
	AccountID     string     `json:"accountId"`
	DisplayName   *string    `json:"displayName,omitempty"`
	AvatarURL     *string    `json:"avatarUrl,omitempty"`
	E2EPublicKey  []byte     `json:"e2ePublicKey,omitempty"`
	LastMessageAt *time.Time `json:"lastMessageAt,omitempty"`
}

type listFriendsResponse struct {
	Friends []friendView `json:"friends"`
}
