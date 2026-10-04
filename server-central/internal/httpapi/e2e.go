package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// e2ePublicKeyLength é o tamanho de uma chave pública X25519 (NaCl box) --
// ver docs/architecture.md, "Decisão: criptografia ponta-a-ponta em DMs".
const e2ePublicKeyLength = 32

// maxE2EKeyBackupLength limita o backup cifrado da chave privada (mesmo teto
// do CHECK da coluna, migrations/0008_e2e_key_backup.up.sql). O formato é do
// client (parâmetros do scrypt, salt, nonce e a chave cifrada); server-central
// não lê nada dele, só o tamanho.
const maxE2EKeyBackupLength = 1024

// PUT /api/me/e2e-public-key — publica a chave pública de E2E do dispositivo
// atual. Caminho de clients anteriores ao backup com frase de recuperação:
// devolve 409 se a conta já tiver backup, para que um client antigo não
// substitua a chave da conta pela de um dispositivo avulso.
func handleSetE2EPublicKey(accounts *store.AccountStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		var body setE2EPublicKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		if len(body.PublicKey) != e2ePublicKeyLength {
			apierr.WriteParams(w, http.StatusBadRequest, "e2e.public_key_invalid", "publicKey precisa ter 32 bytes", apierr.Params{"bytes": e2ePublicKeyLength})
			return
		}

		err := accounts.SetE2EPublicKey(r.Context(), account.ID, body.PublicKey)
		if errors.Is(err, store.ErrConflict) {
			apierr.Write(w, http.StatusConflict, "e2e.recovery_key_exists", "a conta já tem chave com frase de recuperação; atualize o FFCom")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "e2e.public_key_save_failed", "erro ao salvar chave pública")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

type setE2EPublicKeyRequest struct {
	PublicKey []byte `json:"publicKey"`
}

// GET /api/me/e2e-key-backup — devolve a chave pública da conta e o backup
// cifrado da chave privada, para um dispositivo novo desbloquear com a frase
// de recuperação. Só a própria conta lê o próprio backup. 404 se ainda não
// houver backup. Ver docs/architecture.md, "Decisão: backup da chave de E2E
// com frase de recuperação".
func handleGetE2EKeyBackup(accounts *store.AccountStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		publicKey, backup, err := accounts.GetE2EKeyBackup(r.Context(), account.ID)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "e2e.backup_not_found", "conta sem backup de chave")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "e2e.backup_fetch_failed", "erro ao buscar backup de chave")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(e2eKeyBackupResponse{PublicKey: publicKey, Backup: backup})
	})
}

type e2eKeyBackupResponse struct {
	PublicKey []byte `json:"publicKey"`
	Backup    []byte `json:"backup"`
}

// PUT /api/me/e2e-key-backup — grava a chave pública da conta junto com o
// backup cifrado da chave privada. Sem replace, 409 se a conta já tiver
// backup (outro dispositivo criou antes; este deve desbloquear aquele). Com
// replace, troca a chave da conta ("esqueci a frase").
func handleSetE2EKeyBackup(accounts *store.AccountStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		var body setE2EKeyBackupRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		if len(body.PublicKey) != e2ePublicKeyLength {
			apierr.WriteParams(w, http.StatusBadRequest, "e2e.public_key_invalid", "publicKey precisa ter 32 bytes", apierr.Params{"bytes": e2ePublicKeyLength})
			return
		}
		if len(body.Backup) == 0 || len(body.Backup) > maxE2EKeyBackupLength {
			apierr.Write(w, http.StatusBadRequest, "e2e.backup_size_invalid", "backup vazio ou grande demais")
			return
		}

		err := accounts.SetE2EKeyWithBackup(r.Context(), account.ID, body.PublicKey, body.Backup, body.Replace)
		if errors.Is(err, store.ErrConflict) {
			apierr.Write(w, http.StatusConflict, "e2e.backup_exists", "a conta já tem backup de chave")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "e2e.backup_save_failed", "erro ao salvar backup de chave")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

type setE2EKeyBackupRequest struct {
	PublicKey []byte `json:"publicKey"`
	Backup    []byte `json:"backup"`
	Replace   bool   `json:"replace"`
}
