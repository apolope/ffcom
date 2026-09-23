package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/livekit"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// voiceRoomLister é o que voicePresence precisa do LiveKit; interface só
// para o teste trocar por um fake.
type voiceRoomLister interface {
	ListRoomNames(ctx context.Context) ([]string, error)
	ListParticipants(ctx context.Context, room string) ([]livekit.Participant, error)
}

// voicePresence guarda por ttl a última foto de "quem está em cada sala",
// compartilhada entre todos os membros: cada client faz poll de
// GET /api/voice/participants, e sem cache N clients virariam N rodadas de
// chamadas ao LiveKit. A foto é crua (todas as salas); o filtro por
// ViewChannels é feito por requisição, no handler.
type voicePresence struct {
	rooms voiceRoomLister
	ttl   time.Duration

	mu        sync.Mutex
	fetchedAt time.Time
	snapshot  map[string][]livekit.Participant
}

func newVoicePresence(rooms voiceRoomLister, ttl time.Duration) *voicePresence {
	return &voicePresence{rooms: rooms, ttl: ttl}
}

// get devolve a foto atual, buscando de novo se passou do ttl. O lock fica
// preso durante a busca de propósito: requisições simultâneas esperam a
// mesma busca em vez de dispararem uma cada.
func (p *voicePresence) get(ctx context.Context) (map[string][]livekit.Participant, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.snapshot != nil && time.Since(p.fetchedAt) < p.ttl {
		return p.snapshot, nil
	}

	names, err := p.rooms.ListRoomNames(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := make(map[string][]livekit.Participant, len(names))
	for _, name := range names {
		participants, err := p.rooms.ListParticipants(ctx, name)
		if err != nil {
			// Sala pode ter fechado entre as duas chamadas; as outras
			// continuam valendo.
			log.Printf("server-channel: erro ao listar participantes da sala %s: %v", name, err)
			continue
		}
		if len(participants) > 0 {
			snapshot[name] = participants
		}
	}
	p.snapshot = snapshot
	p.fetchedAt = time.Now()
	return snapshot, nil
}

// GET /api/voice/participants — quem está conectado agora em cada canal de
// voz que o membro consegue ver (ViewChannels, mesma regra de
// GET /api/channels), para a barra lateral mostrar a sala antes de entrar.
// Só aparecem canais com alguém dentro. Ver docs/architecture.md, "Decisão:
// participantes da sala de voz na barra lateral".
func handleListVoiceParticipants(presence *voicePresence, channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		rows, err := channels.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao buscar canais", http.StatusInternalServerError)
			return
		}
		visible, err := visibleChannels(r.Context(), roles, overwrites, member, rows)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}

		snapshot, err := presence.get(r.Context())
		if err != nil {
			log.Printf("server-channel: erro ao consultar salas do LiveKit: %v", err)
			http.Error(w, "LiveKit indisponível", http.StatusBadGateway)
			return
		}

		out := make(map[string][]voiceParticipantView)
		for _, c := range visible {
			if c.Type != store.ChannelVoice {
				continue
			}
			participants := snapshot[c.ID]
			if len(participants) == 0 {
				continue
			}
			views := make([]voiceParticipantView, len(participants))
			for i, p := range participants {
				views[i] = voiceParticipantView{MemberID: p.Identity, Name: p.Name}
			}
			out[c.ID] = views
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listVoiceParticipantsResponse{Channels: out})
	})
}

type listVoiceParticipantsResponse struct {
	// Chave é o id do canal de voz.
	Channels map[string][]voiceParticipantView `json:"channels"`
}

// Name é o nome com que a pessoa entrou na sala (apelido na hora do token);
// o client prefere o apelido atual da lista de membros quando tem.
type voiceParticipantView struct {
	MemberID string `json:"memberId"`
	Name     string `json:"name"`
}
