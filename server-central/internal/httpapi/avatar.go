package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/storage"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// multipartMemoryThreshold é o maxMemory passado a ParseMultipartForm: só
// controla quantos bytes de partes não-arquivo ficam em memória antes de
// spillar pra um temp file, não é um limite de tamanho (isso é
// http.MaxBytesReader, aplicado antes, com o valor de avatarMaxBytes).
// Mesmo padrão de server-channel (ver internal/httpapi/attachments.go lá).
const multipartMemoryThreshold = 1 << 20

var allowedAvatarContentTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// POST /api/me/avatar — recebe uma imagem (multipart/form-data, campo
// "file") e a define como avatar da conta autenticada, substituindo
// qualquer avatar anterior. Ver docs/architecture.md, "Decisão: upload de
// avatar de conta".
//
// profiles.display_name é NOT NULL: na primeira vez que uma conta ganha
// avatar sem nunca ter escolhido nome (PUT /api/me/display-name), o valor
// persistido é o oidcSubject cru, marcador de "sem nome escolhido" (ver
// currentOrFallbackDisplayName).
func handleUploadAvatar(profiles *store.ProfileStore, files *storage.AvatarStore, avatarMaxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		// +256KiB de folga sobre avatarMaxBytes para o resto do multipart form
		// (boundary, headers de parte) -- o arquivo em si continua limitado a
		// avatarMaxBytes pelo tamanho declarado no header da parte.
		r.Body = http.MaxBytesReader(w, r.Body, avatarMaxBytes+256<<10)
		if err := r.ParseMultipartForm(multipartMemoryThreshold); err != nil {
			apierr.Write(w, http.StatusRequestEntityTooLarge, "avatar.invalid_or_too_large", "corpo inválido ou avatar maior que o limite permitido")
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			apierr.Write(w, http.StatusBadRequest, "avatar.file_required", "campo \"file\" obrigatório")
			return
		}
		defer file.Close()

		if header.Size > avatarMaxBytes {
			apierr.Write(w, http.StatusRequestEntityTooLarge, "avatar.too_large", "avatar maior que o limite permitido")
			return
		}
		contentType := header.Header.Get("Content-Type")
		if !allowedAvatarContentTypes[contentType] {
			apierr.Write(w, http.StatusBadRequest, "avatar.unsupported_type", "tipo de arquivo não suportado (use PNG, JPEG, WebP ou GIF)")
			return
		}

		if err := files.Save(account.ID, file); err != nil {
			log.Printf("server-central: erro ao salvar avatar: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "avatar.save_failed", "erro ao salvar avatar")
			return
		}

		displayName, err := currentOrFallbackDisplayName(r, profiles, account)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "profile.fetch_failed", "erro ao buscar perfil")
			return
		}

		avatarURL := "/api/avatars/" + account.ID
		if _, err := profiles.Upsert(r.Context(), account.ID, displayName, &avatarURL); err != nil {
			log.Printf("server-central: erro ao gravar avatar_url: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "avatar.save_failed", "erro ao salvar avatar")
			return
		}

		respondMe(w, r, profiles, account)
	})
}

// DELETE /api/me/avatar — remove o avatar da conta autenticada. Não apaga
// a linha de profiles (o display_name provisório continua valendo), só
// zera avatar_url e o arquivo em disco.
func handleDeleteAvatar(profiles *store.ProfileStore, files *storage.AvatarStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		if err := files.Delete(account.ID); err != nil {
			log.Printf("server-central: erro ao apagar avatar: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "avatar.delete_failed", "erro ao remover avatar")
			return
		}

		storedName, err := profiles.StoredDisplayName(r.Context(), account.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusInternalServerError, "profile.fetch_failed", "erro ao buscar perfil")
			return
		}
		if err == nil {
			if _, err := profiles.Upsert(r.Context(), account.ID, storedName, nil); err != nil {
				log.Printf("server-central: erro ao limpar avatar_url: %v", err)
				apierr.Write(w, http.StatusInternalServerError, "avatar.delete_failed", "erro ao remover avatar")
				return
			}
		}

		respondMe(w, r, profiles, account)
	})
}

// GET /api/avatars/{id} — serve os bytes do avatar de qualquer conta.
// Sem checagem de amizade/permissão além de estar autenticado: um avatar
// não é dado sensível, e o id da conta só chega a quem já tem alguma
// relação com ela (amigo, ou membro do mesmo server-channel) -- ver
// docs/architecture.md, "Decisão: upload de avatar de conta". Precisa de
// Bearer token como qualquer outra rota deste servidor, por isso um <img
// src="..."> direto não funciona -- o client sempre busca via
// fetch()+Blob (ver client/src/components/UserAvatar.tsx).
func handleGetAvatar(files *storage.AvatarStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")

		file, err := files.Open(accountID)
		if errors.Is(err, fs.ErrNotExist) {
			apierr.Write(w, http.StatusNotFound, "avatar.not_found", "conta sem avatar")
			return
		}
		if err != nil {
			log.Printf("server-central: erro ao abrir avatar %s: %v", accountID, err)
			apierr.Write(w, http.StatusInternalServerError, "avatar.read_failed", "erro ao ler avatar")
			return
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			log.Printf("server-central: erro ao ler metadados do avatar %s: %v", accountID, err)
			apierr.Write(w, http.StatusInternalServerError, "avatar.read_failed", "erro ao ler avatar")
			return
		}

		// Sem Content-Type gravado à parte -- http.ServeContent sniffa a partir
		// dos primeiros bytes do próprio arquivo (net/http.DetectContentType),
		// já que só os tipos em allowedAvatarContentTypes chegam a ser salvos.
		http.ServeContent(w, r, "", info.ModTime(), file)
	})
}

// currentOrFallbackDisplayName devolve o display_name a regravar junto com o
// avatar novo. Sem perfil ainda, grava o "sub" como marcador de "sem nome
// escolhido": displayNameSQL descarta esse valor e mostra o nome do
// Authentik (accounts.profile_name), que continua acompanhando mudanças de
// nome no Authentik.
func currentOrFallbackDisplayName(r *http.Request, profiles *store.ProfileStore, account store.Account) (string, error) {
	name, err := profiles.StoredDisplayName(r.Context(), account.ID)
	if errors.Is(err, store.ErrNotFound) {
		return account.OIDCSubject, nil
	}
	if err != nil {
		return "", fmt.Errorf("buscar perfil: %w", err)
	}
	return name, nil
}

func respondMe(w http.ResponseWriter, r *http.Request, profiles *store.ProfileStore, account store.Account) {
	resp, err := buildMeResponse(r.Context(), profiles, account)
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "profile.fetch_failed", "erro ao buscar perfil")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
