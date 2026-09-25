package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/storage"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// Criar/renomear/mover/apagar categoria e canal — ver docs/architecture.md,
// "Decisão: gerenciar categorias e canais". Checado só na permissão base
// (sem overwrite por canal): é administração da estrutura do servidor, não
// de um canal específico. ManageChannels vale tudo; cada ação também aceita
// o bit granular dela (CreateChannels, ReorderCategories etc., ver
// docs/permissions.md). Renomear só com ManageChannels.

// maxStructureNameLength limita nome de categoria/canal em caracteres (não
// bytes). Sem limite, um nome gigante quebraria a sidebar de todo membro.
const maxStructureNameLength = 100

// structureBase resolve a permissão base do membro autenticado, com a
// resposta de erro já escrita quando ok é false.
func structureBase(w http.ResponseWriter, r *http.Request, roles *store.RoleStore) (int64, bool) {
	member, ok := auth.MemberFromContext(r.Context())
	if !ok {
		http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
		return 0, false
	}
	base, _, err := memberBasePermission(r.Context(), roles, member)
	if err != nil {
		http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
		return 0, false
	}
	return base, true
}

// allowStructure confere se base tem ManageChannels ou o bit granular da
// ação (bit 0 = só ManageChannels), escrevendo 403 se não.
func allowStructure(w http.ResponseWriter, base, bit int64, bitName string) bool {
	if permissions.Has(base, permissions.ManageChannels|bit) {
		return true
	}
	http.Error(w, "requer a permissão "+bitName, http.StatusForbidden)
	return false
}

// requireStructure junta structureBase e allowStructure para as rotas que
// fazem uma ação só.
func requireStructure(w http.ResponseWriter, r *http.Request, roles *store.RoleStore, bit int64, bitName string) bool {
	base, ok := structureBase(w, r, roles)
	return ok && allowStructure(w, base, bit, bitName)
}

// normalizeStructureName tira espaços das pontas e valida o tamanho,
// devolvendo a mensagem de erro para o client quando inválido.
func normalizeStructureName(raw string) (string, string) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", "name é obrigatório"
	}
	if utf8.RuneCountInString(name) > maxStructureNameLength {
		return "", "name excede 100 caracteres"
	}
	return name, ""
}

// validCategoryID confere que categoryID (quando não nulo) aponta para uma
// categoria existente, respondendo 400 se não. Sem isso, o erro de FK do
// Postgres viraria um 500 genérico.
func validCategoryID(ctx context.Context, w http.ResponseWriter, categories *store.CategoryStore, categoryID *string) bool {
	if categoryID == nil {
		return true
	}
	if _, err := categories.GetByID(ctx, *categoryID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "categoryId não existe", http.StatusBadRequest)
		} else {
			http.Error(w, "erro ao buscar categoria", http.StatusInternalServerError)
		}
		return false
	}
	return true
}

type createCategoryRequest struct {
	Name     string `json:"name"`
	Position *int   `json:"position,omitempty"`
}

// POST /api/categories — cria uma categoria, por padrão no fim da lista.
func handleCreateCategory(categories *store.CategoryStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireStructure(w, r, roles, permissions.CreateCategories, "ManageChannels ou CreateCategories") {
			return
		}
		var body createCategoryRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		name, msg := normalizeStructureName(body.Name)
		if msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		category, err := categories.Create(r.Context(), name, body.Position)
		if err != nil {
			http.Error(w, "erro ao criar categoria", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toCategoryView(category))
	})
}

type updateCategoryRequest struct {
	Name     *string `json:"name,omitempty"`
	Position *int    `json:"position,omitempty"`
}

// PATCH /api/categories/{id} — renomeia (só ManageChannels) e/ou
// reposiciona (ReorderCategories). Campo ausente mantém o valor atual.
func handleUpdateCategory(categories *store.CategoryStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := structureBase(w, r, roles)
		if !ok {
			return
		}
		var body updateCategoryRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		if body.Name != nil && !allowStructure(w, base, 0, "ManageChannels") {
			return
		}
		if body.Name == nil && !allowStructure(w, base, permissions.ReorderCategories, "ManageChannels ou ReorderCategories") {
			return
		}

		existing, err := categories.GetByID(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "categoria não encontrada", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao buscar categoria", http.StatusInternalServerError)
			return
		}

		name, position := existing.Name, existing.Position
		if body.Name != nil {
			var msg string
			if name, msg = normalizeStructureName(*body.Name); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
		}
		if body.Position != nil {
			position = *body.Position
		}

		updated, err := categories.Update(r.Context(), existing.ID, name, position)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "categoria não encontrada", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao atualizar categoria", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toCategoryView(updated))
	})
}

// DELETE /api/categories/{id} — apaga a categoria; os canais dela continuam
// existindo, sem categoria (mesmo comportamento do Discord).
func handleDeleteCategory(categories *store.CategoryStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireStructure(w, r, roles, permissions.DeleteCategories, "ManageChannels ou DeleteCategories") {
			return
		}
		err := categories.Delete(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "categoria não encontrada", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao apagar categoria", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type reorderCategoriesRequest struct {
	IDs []string `json:"ids"`
}

// PUT /api/categories/order — grava a ordem inteira das categorias de uma
// vez (arrastar e soltar no client), em transação, com posições 0..n-1.
// ids precisa ser exatamente o conjunto atual: se alguém criou ou apagou
// uma categoria no meio tempo, 409 e o client recarrega em vez de gravar
// uma ordem montada sobre uma lista velha.
func handleReorderCategories(categories *store.CategoryStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireStructure(w, r, roles, permissions.ReorderCategories, "ManageChannels ou ReorderCategories") {
			return
		}
		var body reorderCategoriesRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		err := categories.Reorder(r.Context(), body.IDs)
		if errors.Is(err, store.ErrOrderMismatch) {
			http.Error(w, "a lista de categorias mudou; recarregue e tente de novo", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "erro ao reordenar categorias", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type reorderChannelsRequest struct {
	Groups []struct {
		CategoryID *string  `json:"categoryId"`
		IDs        []string `json:"ids"`
	} `json:"groups"`
}

// PUT /api/channels/order — grava a ordem dos canais de uma ou mais
// categorias (arrastar e soltar no client; mover entre categorias manda o
// grupo de origem e o de destino). Canais da categoria que não vieram na
// lista vão para o fim dela, ver ChannelStore.Reorder. Canal ou categoria
// apagada no meio tempo dá 409, e o client recarrega e tenta de novo.
func handleReorderChannels(channels *store.ChannelStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireStructure(w, r, roles, permissions.ReorderChannels, "ManageChannels ou ReorderChannels") {
			return
		}
		var body reorderChannelsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Groups) == 0 {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		groups := make([]store.ChannelGroup, len(body.Groups))
		for i, g := range body.Groups {
			groups[i] = store.ChannelGroup{CategoryID: g.CategoryID, IDs: g.IDs}
		}
		err := channels.Reorder(r.Context(), groups)
		if errors.Is(err, store.ErrChannelOrderInvalid) {
			http.Error(w, "a lista de canais mudou; recarregue e tente de novo", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "erro ao reordenar canais", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type createChannelRequest struct {
	CategoryID *string `json:"categoryId,omitempty"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Position   *int    `json:"position,omitempty"`
}

// POST /api/channels — cria um canal de texto, voz ou forum, por padrão no
// fim da categoria (ou entre os sem categoria, com categoryId nulo).
func handleCreateChannel(categories *store.CategoryStore, channels *store.ChannelStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireStructure(w, r, roles, permissions.CreateChannels, "ManageChannels ou CreateChannels") {
			return
		}
		var body createChannelRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		name, msg := normalizeStructureName(body.Name)
		if msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		channelType := store.ChannelType(body.Type)
		switch channelType {
		case store.ChannelText, store.ChannelVoice, store.ChannelForum:
		default:
			http.Error(w, "type deve ser text, voice ou forum", http.StatusBadRequest)
			return
		}
		if !validCategoryID(r.Context(), w, categories, body.CategoryID) {
			return
		}

		channel, err := channels.Create(r.Context(), body.CategoryID, name, channelType, body.Position)
		if err != nil {
			http.Error(w, "erro ao criar canal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toChannelView(channel))
	})
}

// PATCH /api/channels/{id} — renomeia (só ManageChannels), move de
// categoria e/ou reposiciona (ReorderChannels). Campo ausente mantém o
// valor atual; "categoryId": null tira o canal da
// categoria. Por isso o corpo é lido como mapa: com um *string comum não dá
// pra distinguir campo ausente de null. O tipo do canal não muda.
func handleUpdateChannel(categories *store.CategoryStore, channels *store.ChannelStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := structureBase(w, r, roles)
		if !ok {
			return
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		_, renames := body["name"]
		if renames && !allowStructure(w, base, 0, "ManageChannels") {
			return
		}
		if !renames && !allowStructure(w, base, permissions.ReorderChannels, "ManageChannels ou ReorderChannels") {
			return
		}

		existing, err := channels.GetByID(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao buscar canal", http.StatusInternalServerError)
			return
		}

		name, categoryID, position := existing.Name, existing.CategoryID, existing.Position
		if raw, ok := body["name"]; ok {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				http.Error(w, "name inválido", http.StatusBadRequest)
				return
			}
			var msg string
			if name, msg = normalizeStructureName(value); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
		}
		if raw, ok := body["categoryId"]; ok {
			categoryID = nil
			if err := json.Unmarshal(raw, &categoryID); err != nil {
				http.Error(w, "categoryId inválido", http.StatusBadRequest)
				return
			}
			if !validCategoryID(r.Context(), w, categories, categoryID) {
				return
			}
		}
		if raw, ok := body["position"]; ok {
			if err := json.Unmarshal(raw, &position); err != nil {
				http.Error(w, "position inválido", http.StatusBadRequest)
				return
			}
		}
		if _, ok := body["type"]; ok {
			http.Error(w, "o tipo do canal não pode ser alterado", http.StatusBadRequest)
			return
		}

		updated, err := channels.Update(r.Context(), existing.ID, name, categoryID, position)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao atualizar canal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toChannelView(updated))
	})
}

// DELETE /api/channels/{id} — apaga o canal com todo o conteúdo: mensagens,
// threads e anexos. As linhas vão por CASCADE; os arquivos de anexo em
// disco são buscados antes e apagados depois, do mesmo jeito que ao apagar
// uma mensagem (handleIncomingMessageDelete). Falha ao apagar um arquivo só
// é logada: o canal já sumiu e o arquivo órfão não é acessível por nenhuma
// rota.
func handleDeleteChannel(channels *store.ChannelStore, attachments *store.AttachmentStore, files *storage.FileStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireStructure(w, r, roles, permissions.DeleteChannels, "ManageChannels ou DeleteChannels") {
			return
		}
		channelID := r.PathValue("id")

		keys, err := attachments.StorageKeysForChannel(r.Context(), channelID)
		if err != nil {
			http.Error(w, "erro ao buscar anexos do canal", http.StatusInternalServerError)
			return
		}

		err = channels.Delete(r.Context(), channelID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao apagar canal", http.StatusInternalServerError)
			return
		}

		for _, key := range keys {
			if err := files.Delete(key); err != nil {
				log.Printf("server-channel: erro ao apagar arquivo de anexo %s do canal %s: %v", key, channelID, err)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
