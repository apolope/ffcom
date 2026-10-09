// Package push avisa o server-central de mensagens novas, para ele mandar
// notificação push aos aparelhos de quem deu um grant a este servidor. Ver
// docs/architecture.md, "Decisão: notificações push (fase 6)", e a rota
// POST /api/push/notify do server-central em docs/protocol.md.
//
// Tudo é best-effort e fora do caminho da mensagem: quem cria a mensagem só
// enfileira; os destinatários são calculados e o central é chamado numa
// goroutine própria. Central fora do ar, lento ou recusando só gera log.
// Nada do que vai ao central fica guardado aqui além do que já existe (a
// mensagem e o grant de cada membro).
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultCentralURL é a instância oficial do server-central, usada quando
// FFCOM_CENTRAL_URL está vazia.
const DefaultCentralURL = "https://central.ffcom.a3sitsolutions.com.br"

const (
	// maxGrantsPerRequest acompanha o limite do server-central por chamada.
	maxGrantsPerRequest = 500
	// maxTextRunes: o central corta em 300; aqui só evita mandar 4000.
	maxTextRunes   = 500
	queueSize      = 1000
	requestTimeout = 10 * time.Second
)

// Message é uma mensagem nova, como sai do handler que a criou.
type Message struct {
	ChannelID      string
	MessageID      string
	ThreadID       string
	ThreadTitle    string
	AuthorMemberID string
	Author         string
	// AuthorSubject é o "sub" do Authentik do autor (store.Member.OIDCSubject),
	// que o server-central usa para achar o avatar da conta e mostrá-lo na
	// notificação.
	AuthorSubject string
	Text           string
	Attachment     string
	// Viewers são os membros com o WebSocket do canal aberto no momento
	// em que a mensagem foi criada: já estão vendo, não recebem push.
	Viewers map[string]bool
}

// Recipient é um grant de quem deve ser notificado, com o endereço deste
// servidor como aquela pessoa o conhece.
type Recipient struct {
	Grant         string
	ServerAddress string
}

// Resolver calcula o nome do canal e os destinatários de uma mensagem
// (internal/httpapi monta a partir do banco e das permissões).
type Resolver func(ctx context.Context, m Message) (channelName string, recipients []Recipient, err error)

// Notifier é a fila até o server-central. Um *Notifier nil é o push
// desligado (FFCOM_CENTRAL_URL=off): Enqueue vira no-op.
type Notifier struct {
	notifyURL string
	resolve   Resolver
	http      *http.Client
	queue     chan Message
	pending   sync.WaitGroup
}

// New cria o Notifier que chama centralURL e sobe o worker. centralURL
// vazia usa DefaultCentralURL; "off" desliga o push e devolve nil.
func New(centralURL string, resolve Resolver) *Notifier {
	centralURL = strings.TrimSpace(centralURL)
	if strings.EqualFold(centralURL, "off") {
		return nil
	}
	if centralURL == "" {
		centralURL = DefaultCentralURL
	}
	n := &Notifier{
		notifyURL: strings.TrimRight(centralURL, "/") + "/api/push/notify",
		resolve:   resolve,
		http:      &http.Client{Timeout: requestTimeout},
		queue:     make(chan Message, queueSize),
	}
	go n.run()
	return n
}

// Enqueue põe a mensagem na fila sem bloquear. Fila cheia (central muito
// lento) descarta com log.
func (n *Notifier) Enqueue(m Message) {
	if n == nil {
		return
	}
	n.pending.Add(1)
	select {
	case n.queue <- m:
	default:
		n.pending.Done()
		log.Printf("server-channel: fila de push cheia, notificação descartada")
	}
}

// Wait espera a fila esvaziar. Só para testes.
func (n *Notifier) Wait() {
	if n != nil {
		n.pending.Wait()
	}
}

func (n *Notifier) run() {
	for m := range n.queue {
		n.process(m)
		n.pending.Done()
	}
}

func (n *Notifier) process(m Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	channelName, recipients, err := n.resolve(ctx, m)
	if err != nil {
		log.Printf("server-channel: push: calcular destinatários: %v", err)
		return
	}
	if len(recipients) == 0 {
		return
	}

	// Um lote por endereço: o grant só vale no endereço em que foi emitido,
	// e quase sempre todos usam o mesmo.
	byAddress := make(map[string][]string)
	for _, r := range recipients {
		byAddress[r.ServerAddress] = append(byAddress[r.ServerAddress], r.Grant)
	}
	for address, grants := range byAddress {
		for start := 0; start < len(grants); start += maxGrantsPerRequest {
			end := min(start+maxGrantsPerRequest, len(grants))
			body := notifyRequest{
				ServerAddress: address,
				ChannelID:     m.ChannelID,
				ChannelName:   channelName,
				Author:        m.Author,
				AuthorSubject: m.AuthorSubject,
				Text:          truncate(m.Text, maxTextRunes),
				MessageID:     m.MessageID,
				ThreadID:      m.ThreadID,
				ThreadTitle:   m.ThreadTitle,
				Attachment:    m.Attachment,
				Grants:        grants[start:end],
			}
			if err := n.post(ctx, body); err != nil {
				log.Printf("server-channel: push: %v", err)
			}
		}
	}
}

func (n *Notifier) post(ctx context.Context, body notifyRequest) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.notifyURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.http.Do(req)
	if err != nil {
		return fmt.Errorf("chamar o server-central: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("server-central respondeu %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// notifyRequest é o corpo de POST /api/push/notify do server-central.
type notifyRequest struct {
	ServerAddress string   `json:"serverAddress"`
	ChannelID     string   `json:"channelId"`
	ChannelName   string   `json:"channelName"`
	Author        string   `json:"author"`
	Text          string   `json:"text"`
	MessageID     string   `json:"messageId,omitempty"`
	ThreadID      string   `json:"threadId,omitempty"`
	ThreadTitle   string   `json:"threadTitle,omitempty"`
	Attachment    string   `json:"attachment,omitempty"`
	AuthorSubject string   `json:"authorSubject,omitempty"`
	Grants        []string `json:"grants"`
}
