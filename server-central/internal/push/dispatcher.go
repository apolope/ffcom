package push

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// Devices é o que o Dispatcher precisa do banco (store.PushStore).
type Devices interface {
	DeviceTokens(ctx context.Context, accountID string) ([]string, error)
	DeleteDeviceToken(ctx context.Context, token string) error
}

// Builder monta os dados de uma notificação na hora de enviar (ex. buscar o
// nome de quem mandou a DM fora do caminho da requisição). Devolver nil
// cancela o envio.
type Builder func(ctx context.Context) map[string]string

// AuthorAvatars dá os campos authorAvatar (link do avatar, sem login) e
// authorKey (versão estável do avatar, chave do cache do app) de quem
// mandou a notificação. url vazia: sem avatar, e os dois campos ficam de
// fora. Implementado por httpapi.PushAvatarLinks.
type AuthorAvatars interface {
	ForAccount(ctx context.Context, accountID string) (url, key string)
	ForSubject(ctx context.Context, subject string) (url, key string)
}

type job struct {
	accountID string
	build     Builder
}

// Dispatcher entrega notificações em segundo plano: a rota ou o WebSocket
// que gera a notificação só enfileira, e nunca espera o FCM. Fila cheia
// descarta (com log) em vez de segurar quem chamou.
//
// Um *Dispatcher nil é o push desligado (sem FCM_SERVICE_ACCOUNT_JSON):
// todos os métodos viram no-op.
type Dispatcher struct {
	sender  Sender
	devices Devices
	avatars AuthorAvatars
	jobs    chan job
	pending sync.WaitGroup
}

// NewDispatcher sobe workers goroutines lendo uma fila de queueSize.
func NewDispatcher(sender Sender, devices Devices, workers, queueSize int) *Dispatcher {
	d := &Dispatcher{sender: sender, devices: devices, jobs: make(chan job, queueSize)}
	for range workers {
		go d.run()
	}
	return d
}

// SetAuthorAvatars liga o avatar de quem mandou nas notificações. Chamar
// antes do primeiro Notify.
func (d *Dispatcher) SetAuthorAvatars(a AuthorAvatars) {
	if d != nil {
		d.avatars = a
	}
}

// AddAuthorAvatar põe em data os campos authorAvatar e authorKey da conta
// accountID ou, sem ela, do "sub" subject. Sem avatar, sem AuthorAvatars ou
// com erro, data fica como está (o app mostra a letra).
func (d *Dispatcher) AddAuthorAvatar(ctx context.Context, data map[string]string, accountID, subject string) {
	if d == nil || d.avatars == nil {
		return
	}
	var url, key string
	switch {
	case accountID != "":
		url, key = d.avatars.ForAccount(ctx, accountID)
	case subject != "":
		url, key = d.avatars.ForSubject(ctx, subject)
	}
	if url == "" {
		return
	}
	data["authorAvatar"] = url
	if key != "" {
		data["authorKey"] = key
	}
}

// Enabled diz se o push está ligado.
func (d *Dispatcher) Enabled() bool { return d != nil }

// Notify enfileira data para todos os aparelhos de accountID.
func (d *Dispatcher) Notify(accountID string, data map[string]string) {
	d.NotifyFunc(accountID, func(context.Context) map[string]string { return data })
}

// NotifyFunc é Notify com os dados montados pelo worker.
func (d *Dispatcher) NotifyFunc(accountID string, build Builder) {
	if d == nil {
		return
	}
	d.pending.Add(1)
	select {
	case d.jobs <- job{accountID: accountID, build: build}:
	default:
		d.pending.Done()
		log.Printf("server-central: fila de push cheia, notificação descartada")
	}
}

// Wait espera a fila esvaziar. Só para testes.
func (d *Dispatcher) Wait() {
	if d != nil {
		d.pending.Wait()
	}
}

func (d *Dispatcher) run() {
	for j := range d.jobs {
		d.deliver(j)
		d.pending.Done()
	}
}

func (d *Dispatcher) deliver(j job) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data := j.build(ctx)
	if data == nil {
		return
	}
	tokens, err := d.devices.DeviceTokens(ctx, j.accountID)
	if err != nil {
		log.Printf("server-central: push: buscar aparelhos: %v", err)
		return
	}
	for _, token := range tokens {
		err := d.sender.Send(ctx, token, data)
		if errors.Is(err, ErrUnregistered) {
			if err := d.devices.DeleteDeviceToken(ctx, token); err != nil {
				log.Printf("server-central: push: apagar token não registrado: %v", err)
			}
			continue
		}
		if err != nil {
			log.Printf("server-central: push: %v", err)
		}
	}
}
