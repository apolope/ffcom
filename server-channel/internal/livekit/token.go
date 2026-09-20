// Package livekit assina access tokens (JWT) para o SFU LiveKit. Não usa o
// SDK oficial github.com/livekit/server-sdk-go: esse módulo reexporta
// github.com/livekit/protocol, que arrasta pion/webrtc, redis e prometheus
// só para montar um JWT — peso incompatível com "server-channel é um
// binário Go único fácil de distribuir" (ver docs/architecture.md, decisão
// de linguagem). O formato do token é documentado e estável
// (https://docs.livekit.io/home/get-started/authentication/), então
// assinamos com golang-jwt/jwt diretamente.
package livekit

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// tokenTTL curto o suficiente para não sobreviver muito além de uma sessão
// de chamada; o client pede um novo token a cada tentativa de entrar num
// canal de voz (ver internal/httpapi/voice.go), não precisa durar mais que isso.
const tokenTTL = 6 * time.Hour

// videoGrant é o subconjunto de permissões do LiveKit usado pelo FFCom: só
// entrar na sala do canal de voz e publicar/receber áudio e vídeo. O
// sistema de permissões/roles do FFCom (TODO.md, "Sistema de
// permissões/roles por servidor e por canal") ainda não existe, então por
// ora todo membro autenticado recebe o mesmo grant para qualquer canal de
// voz — ver docs/architecture.md, "Decisão: integração de voz com LiveKit".
type videoGrant struct {
	Room           string `json:"room"`
	RoomJoin       bool   `json:"roomJoin"`
	CanPublish     bool   `json:"canPublish"`
	CanSubscribe   bool   `json:"canSubscribe"`
	CanPublishData bool   `json:"canPublishData"`
}

type accessTokenClaims struct {
	jwt.RegisteredClaims
	Name  string     `json:"name,omitempty"`
	Video videoGrant `json:"video"`
}

// NewAccessToken assina um access token de LiveKit para identity (o
// member.ID de server-channel) entrar na sala room (o id do canal de voz).
// apiKey/apiSecret são o par único configurado para esta instância de
// server-channel (ver docker-compose.yml).
func NewAccessToken(apiKey, apiSecret, identity, displayName, room string) (string, error) {
	now := time.Now()
	claims := accessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    apiKey,
			Subject:   identity,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
		Name: displayName,
		Video: videoGrant{
			Room:           room,
			RoomJoin:       true,
			CanPublish:     true,
			CanSubscribe:   true,
			CanPublishData: true,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(apiSecret))
}
