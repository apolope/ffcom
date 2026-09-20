package httpapi

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// requireManageInvites resolve a permissão base do membro autenticado e
// devolve false (já com a resposta HTTP escrita) se ele não tiver
// ManageInvites nem for dono do servidor.
func requireManageInvites(w http.ResponseWriter, r *http.Request, roles *store.RoleStore) (store.Member, bool) {
	member, ok := auth.MemberFromContext(r.Context())
	if !ok {
		http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
		return store.Member{}, false
	}
	base, _, err := memberBasePermission(r.Context(), roles, member)
	if err != nil {
		http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
		return store.Member{}, false
	}
	if !permissions.Has(base, permissions.ManageInvites) {
		http.Error(w, "requer a permissão ManageInvites", http.StatusForbidden)
		return store.Member{}, false
	}
	return member, true
}

// inviteCodeLength/generateInviteCode seguem o mesmo formato (base32 sem
// padding, 10 caracteres) já usado pelo convite de amizade em server-central
// — evita os caracteres ambíguos 0/O e 1/I de um código pra digitar/colar.
const inviteCodeLength = 10

func generateInviteCode() (string, error) {
	buf := make([]byte, inviteCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return encoded[:inviteCodeLength], nil
}

type createInviteRequest struct {
	MaxUses   *int       `json:"maxUses,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type inviteView struct {
	ID                string     `json:"id"`
	Code              string     `json:"code"`
	CreatedByMemberID string     `json:"createdByMemberId"`
	MaxUses           *int       `json:"maxUses,omitempty"`
	Uses              int        `json:"uses"`
	ExpiresAt         *time.Time `json:"expiresAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

func toInviteView(i store.Invite) inviteView {
	return inviteView{
		ID:                i.ID,
		Code:              i.Code,
		CreatedByMemberID: i.CreatedByMemberID,
		MaxUses:           i.MaxUses,
		Uses:              i.Uses,
		ExpiresAt:         i.ExpiresAt,
		CreatedAt:         i.CreatedAt,
	}
}

// POST /api/invites — gera um novo código de convite para este server-channel
// (ver docs/architecture.md, "Convites obrigatórios para entrar em
// server-channel"). Requer ManageInvites.
func handleCreateInvite(invites *store.InviteStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := requireManageInvites(w, r, roles)
		if !ok {
			return
		}

		var body createInviteRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		if body.MaxUses != nil && *body.MaxUses <= 0 {
			http.Error(w, "maxUses deve ser positivo", http.StatusBadRequest)
			return
		}

		var invite store.Invite
		// Colisão de código é astronomicamente improvável (10 chars base32 =
		// 50 bits de entropia), mas o retry mantém o endpoint correto mesmo
		// assim em vez de assumir unicidade (mesmo padrão do convite de
		// amizade em server-central).
		for attempt := 0; attempt < 5; attempt++ {
			code, err := generateInviteCode()
			if err != nil {
				http.Error(w, "erro ao gerar código de convite", http.StatusInternalServerError)
				return
			}
			invite, err = invites.Create(r.Context(), code, member.ID, body.MaxUses, body.ExpiresAt)
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
		json.NewEncoder(w).Encode(toInviteView(invite))
	})
}

// GET /api/invites — lista todos os convites deste server-channel. Requer
// ManageInvites.
func handleListInvites(invites *store.InviteStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireManageInvites(w, r, roles); !ok {
			return
		}

		rows, err := invites.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao listar convites", http.StatusInternalServerError)
			return
		}
		out := make([]inviteView, len(rows))
		for i, inv := range rows {
			out[i] = toInviteView(inv)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listInvitesResponse{Invites: out})
	})
}

type listInvitesResponse struct {
	Invites []inviteView `json:"invites"`
}

// DELETE /api/invites/{id} — revoga um convite. Requer ManageInvites (não
// mais restrito a quem criou o convite, ver store.InviteStore.Delete).
func handleDeleteInvite(invites *store.InviteStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireManageInvites(w, r, roles); !ok {
			return
		}

		id := r.PathValue("id")
		if err := invites.Delete(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "convite não encontrado", http.StatusNotFound)
				return
			}
			http.Error(w, "erro ao revogar convite", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
