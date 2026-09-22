package httpapi

import (
	"encoding/json"
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// e2ePublicKeyLength é o tamanho de uma chave pública X25519 (NaCl box) --
// ver docs/architecture.md, "Decisão: criptografia ponta-a-ponta em DMs".
const e2ePublicKeyLength = 32

// PUT /api/me/e2e-public-key — publica a chave pública de E2E do dispositivo
// atual da conta autenticada, usada pelos amigos para cifrar DMs endereçadas
// a esta conta. Sobrescreve qualquer chave publicada antes (um dispositivo
// "ativo" por vez nesta v1, sem sincronização multi-dispositivo).
func handleSetE2EPublicKey(accounts *store.AccountStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		var body setE2EPublicKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
			return
		}
		if len(body.PublicKey) != e2ePublicKeyLength {
			http.Error(w, "publicKey precisa ter 32 bytes", http.StatusBadRequest)
			return
		}

		if err := accounts.SetE2EPublicKey(r.Context(), account.ID, body.PublicKey); err != nil {
			http.Error(w, "erro ao salvar chave pública", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

type setE2EPublicKeyRequest struct {
	PublicKey []byte `json:"publicKey"`
}
