package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/push"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// Notificações push (ver docs/architecture.md, "Decisão: notificações push
// (fase 6)"). O app pede ao server-central um grant para este servidor e o
// entrega aqui; a cada mensagem nova, este servidor manda ao central os
// grants de quem pode ver o canal, menos o autor e quem está vendo o canal
// (com a página visível, em foco e sem ausência). O central confere o grant, o silêncio e manda ao Firebase.

const (
	maxPushGrantLength     = 128
	maxServerAddressLength = 512
)

// PUT /api/me/push-grant: guarda o grant de push do membro (troca o
// anterior). serverAddress é o endereço deste servidor como o app o
// conhece, o mesmo do known_servers da conta, ao qual o grant está preso.
func handleSetPushGrant(members *store.MemberStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
			return
		}
		var body pushGrantRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		token := strings.TrimSpace(body.Token)
		if token == "" || len(token) > maxPushGrantLength {
			apierr.Write(w, http.StatusBadRequest, "push.grant_invalid", "token de notificação inválido")
			return
		}
		address := strings.TrimSpace(body.ServerAddress)
		if address == "" || len(address) > maxServerAddressLength {
			apierr.Write(w, http.StatusBadRequest, "push.server_address_required", "serverAddress é obrigatório")
			return
		}
		if err := members.SetPushGrant(r.Context(), member.ID, token, address); err != nil {
			log.Printf("server-channel: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.grant_save_failed", "erro ao salvar o token de notificação")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// DELETE /api/me/push-grant: apaga o grant do membro. Com {token} no
// corpo, só apaga se for o guardado (outro aparelho pode ter entregue um
// mais novo); sem corpo, apaga o que houver.
func handleDeletePushGrant(members *store.MemberStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
			return
		}
		var body pushGrantRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil && err != io.EOF {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		var token *string
		if t := strings.TrimSpace(body.Token); t != "" {
			token = &t
		}
		if err := members.DeletePushGrant(r.Context(), member.ID, token); err != nil {
			log.Printf("server-channel: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.grant_delete_failed", "erro ao remover o token de notificação")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type pushGrantRequest struct {
	Token         string `json:"token"`
	ServerAddress string `json:"serverAddress"`
}

// NewPushResolver calcula os destinatários de uma mensagem: membros ativos
// com grant, menos o autor, menos quem está vendo o canal (m.Viewers), e só quem tem ViewChannels no canal (roles, @everyone e overwrites, a
// mesma conta de channelPermission, feita de uma vez para todos).
func NewPushResolver(db *store.Store) push.Resolver {
	return func(ctx context.Context, m push.Message) (push.Resolution, error) {
		channel, err := db.Channels.GetByID(ctx, m.ChannelID)
		if err != nil {
			return push.Resolution{}, err
		}
		candidates, err := db.Members.PushCandidates(ctx, m.AuthorMemberID)
		if err != nil {
			return push.Resolution{}, err
		}
		res := push.Resolution{ChannelName: channel.Name, Considered: len(candidates)}
		pending := candidates[:0]
		ids := make([]string, 0, len(candidates))
		for _, c := range candidates {
			if m.Viewers[c.Member.ID] {
				res.Viewing++
				continue
			}
			pending = append(pending, c)
			ids = append(ids, c.Member.ID)
		}
		if len(pending) == 0 {
			return res, nil
		}

		allRoles, err := db.Roles.List(ctx)
		if err != nil {
			return push.Resolution{}, err
		}
		rolePerms := make(map[string]int64, len(allRoles))
		everyoneRoleID := ""
		for _, role := range allRoles {
			rolePerms[role.ID] = role.Permissions
			if role.IsDefault {
				everyoneRoleID = role.ID
			}
		}
		assignments, err := db.Roles.AssignmentsForMembers(ctx, ids)
		if err != nil {
			return push.Resolution{}, err
		}
		overwriteRows, err := db.ChannelOverwrites.ListForChannel(ctx, m.ChannelID)
		if err != nil {
			return push.Resolution{}, err
		}
		overwrites := toOverwriteList(overwriteRows)

		for _, c := range pending {
			if !c.Member.IsOwner {
				roleIDs := append(append([]string{}, assignments[c.Member.ID]...), everyoneRoleID)
				perms := make([]int64, len(roleIDs))
				for i, id := range roleIDs {
					perms[i] = rolePerms[id]
				}
				effective := permissions.Effective(permissions.Base(perms), everyoneRoleID, roleIDs, overwrites)
				if !permissions.Has(effective, permissions.ViewChannels) {
					continue
				}
			}
			res.Recipients = append(res.Recipients, push.Recipient{Grant: c.Token, ServerAddress: c.ServerAddress})
		}
		return res, nil
	}
}

// messagePusher devolve o que os handlers de criação de mensagem chamam
// depois do broadcast: enfileira a mensagem no Notifier com quem está vendo
// o canal agora. Com o push desligado (notifier nil), não faz nada.
func messagePusher(notifier *push.Notifier, hub *realtime.Hub, author store.Member) func(m store.Message, threadTitle, attachment string) {
	return func(m store.Message, threadTitle, attachment string) {
		if notifier == nil {
			return
		}
		msg := push.Message{
			ChannelID:      m.ChannelID,
			MessageID:      m.ID,
			ThreadTitle:    threadTitle,
			AuthorMemberID: author.ID,
			Author:         author.DisplayName(),
			AuthorSubject:  author.OIDCSubject,
			Text:           m.Content,
			Attachment:     attachment,
			Viewers:        hub.ViewingMemberIDs(m.ChannelID),
		}
		if m.ThreadID != nil {
			msg.ThreadID = *m.ThreadID
		}
		notifier.Enqueue(msg)
	}
}
