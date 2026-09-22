package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"

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
// profiles.display_name é NOT NULL, mas não existe (ainda) endpoint para
// editá-lo (ver TODO.md) — na primeira vez que uma conta ganha avatar sem
// nunca ter preenchido perfil, o nome de exibição persistido é o
// oidcSubject cru, mesmo fallback que o client já aplica no lugar de
// displayName ausente (ver client/src/hooks/useFriends.ts).
func handleUploadAvatar(profiles *store.ProfileStore, files *storage.AvatarStore, avatarMaxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		// +256KiB de folga sobre avatarMaxBytes para o resto do multipart form
		// (boundary, headers de parte) -- o arquivo em si continua limitado a
		// avatarMaxBytes pelo tamanho declarado no header da parte.
		r.Body = http.MaxBytesReader(w, r.Body, avatarMaxBytes+256<<10)
		if err := r.ParseMultipartForm(multipartMemoryThreshold); err != nil {
			http.Error(w, "corpo inválido ou avatar maior que o limite permitido", http.StatusRequestEntityTooLarge)
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "campo \"file\" obrigatório", http.StatusBadRequest)
			return
		}
		defer file.Close()

		if header.Size > avatarMaxBytes {
			http.Error(w, "avatar maior que o limite permitido", http.StatusRequestEntityTooLarge)
			return
		}
		contentType := header.Header.Get("Content-Type")
		if !allowedAvatarContentTypes[contentType] {
			http.Error(w, "tipo de arquivo não suportado (use PNG, JPEG, WebP ou GIF)", http.StatusBadRequest)
			return
		}

		if err := files.Save(account.ID, file); err != nil {
			log.Printf("server-central: erro ao salvar avatar: %v", err)
			http.Error(w, "erro ao salvar avatar", http.StatusInternalServerError)
			return
		}

		displayName, err := currentOrFallbackDisplayName(r, profiles, account)
		if err != nil {
			http.Error(w, "erro ao buscar perfil", http.StatusInternalServerError)
			return
		}

		avatarURL := "/api/avatars/" + account.ID
		if _, err := profiles.Upsert(r.Context(), account.ID, displayName, &avatarURL); err != nil {
			log.Printf("server-central: erro ao gravar avatar_url: %v", err)
			http.Error(w, "erro ao salvar avatar", http.StatusInternalServerError)
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
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		if err := files.Delete(account.ID); err != nil {
			log.Printf("server-central: erro ao apagar avatar: %v", err)
			http.Error(w, "erro ao remover avatar", http.StatusInternalServerError)
			return
		}

		profile, err := profiles.GetByAccountID(r.Context(), account.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			http.Error(w, "erro ao buscar perfil", http.StatusInternalServerError)
			return
		}
		if err == nil {
			if _, err := profiles.Upsert(r.Context(), account.ID, profile.DisplayName, nil); err != nil {
				log.Printf("server-central: erro ao limpar avatar_url: %v", err)
				http.Error(w, "erro ao remover avatar", http.StatusInternalServerError)
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
			http.Error(w, "conta sem avatar", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("server-central: erro ao abrir avatar %s: %v", accountID, err)
			http.Error(w, "erro ao ler avatar", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			log.Printf("server-central: erro ao ler metadados do avatar %s: %v", accountID, err)
			http.Error(w, "erro ao ler avatar", http.StatusInternalServerError)
			return
		}

		// Sem Content-Type gravado à parte -- http.ServeContent sniffa a partir
		// dos primeiros bytes do próprio arquivo (net/http.DetectContentType),
		// já que só os tipos em allowedAvatarContentTypes chegam a ser salvos.
		http.ServeContent(w, r, "", info.ModTime(), file)
	})
}

func currentOrFallbackDisplayName(r *http.Request, profiles *store.ProfileStore, account store.Account) (string, error) {
	profile, err := profiles.GetByAccountID(r.Context(), account.ID)
	if errors.Is(err, store.ErrNotFound) {
		return account.OIDCSubject, nil
	}
	if err != nil {
		return "", fmt.Errorf("buscar perfil: %w", err)
	}
	return profile.DisplayName, nil
}

func respondMe(w http.ResponseWriter, r *http.Request, profiles *store.ProfileStore, account store.Account) {
	resp, err := buildMeResponse(r.Context(), profiles, account)
	if err != nil {
		http.Error(w, "erro ao buscar perfil", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
