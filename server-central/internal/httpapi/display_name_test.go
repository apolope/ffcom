package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
)

func TestDisplayName(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.Handle("GET /api/me", handleMe(db))
	mux.Handle("PUT /api/me/display-name", handleSetDisplayName(db.Profiles))

	authentikName := "Alice Souza"
	subject := fmt.Sprintf("alice-%s-%d", t.Name(), time.Now().UnixNano())
	alice, err := db.Accounts.GetOrCreateBySubject(ctx, subject, &authentikName)
	if err != nil {
		t.Fatal(err)
	}

	call := func(method string, body any) (int, meResponse) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, map[string]string{
			http.MethodGet: "/api/me",
			http.MethodPut: "/api/me/display-name",
		}[method], bytes.NewReader(raw))
		req = req.WithContext(auth.WithAccount(req.Context(), alice))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var me meResponse
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
				t.Fatalf("resposta não é JSON: %s", rec.Body.String())
			}
		}
		return rec.Code, me
	}
	name := func(me meResponse) string {
		if me.DisplayName == nil {
			return "<nil>"
		}
		return *me.DisplayName
	}

	// Sem nome escolhido: o do Authentik, sem customDisplayName.
	if code, me := call(http.MethodGet, nil); code != http.StatusOK || name(me) != authentikName || me.CustomDisplayName != nil {
		t.Fatalf("inicial: %d %q custom=%v", code, name(me), me.CustomDisplayName)
	}

	// Avatar já gravado precisa sobreviver à troca de nome.
	avatar := "/api/avatars/" + alice.ID
	if _, err := db.Profiles.Upsert(ctx, alice.ID, alice.OIDCSubject, &avatar); err != nil {
		t.Fatal(err)
	}

	code, me := call(http.MethodPut, map[string]any{"displayName": "  Lili  "})
	if code != http.StatusOK || name(me) != "Lili" || me.CustomDisplayName == nil || *me.CustomDisplayName != "Lili" {
		t.Fatalf("definir: %d %q custom=%v", code, name(me), me.CustomDisplayName)
	}
	if me.AvatarURL == nil || *me.AvatarURL != avatar {
		t.Fatalf("avatar perdido ao trocar o nome: %v", me.AvatarURL)
	}

	// Quem vê a conta de fora (amigos, lista de membros) recebe o nome novo.
	summaries, err := db.Accounts.GetManyBySubjects(ctx, []string{alice.OIDCSubject})
	if err != nil || len(summaries) != 1 || summaries[0].DisplayName == nil || *summaries[0].DisplayName != "Lili" {
		t.Fatalf("lookup: %v %+v", err, summaries)
	}

	for _, bad := range []string{strings.Repeat("a", 65), "linha\nquebrada"} {
		if code, _ := call(http.MethodPut, map[string]any{"displayName": bad}); code != http.StatusBadRequest {
			t.Fatalf("nome inválido %q: %d, esperava 400", bad, code)
		}
	}
	// 64 bytes em acentuados (32 letras) cabe.
	if code, _ := call(http.MethodPut, map[string]any{"displayName": strings.Repeat("é", 32)}); code != http.StatusOK {
		t.Fatalf("nome de 64 bytes: %d", code)
	}

	// Vazio e null voltam ao nome do Authentik, mantendo o avatar.
	for _, reset := range []any{map[string]any{"displayName": "   "}, map[string]any{"displayName": nil}} {
		code, me := call(http.MethodPut, reset)
		if code != http.StatusOK || name(me) != authentikName || me.CustomDisplayName != nil {
			t.Fatalf("voltar ao Authentik com %v: %d %q custom=%v", reset, code, name(me), me.CustomDisplayName)
		}
		if me.AvatarURL == nil || *me.AvatarURL != avatar {
			t.Fatalf("avatar perdido ao limpar o nome: %v", me.AvatarURL)
		}
	}

	// O nome do Authentik volta a acompanhar mudanças feitas lá.
	renamed := "Alice S. Souza"
	if _, err := db.Accounts.GetOrCreateBySubject(ctx, subject, &renamed); err != nil {
		t.Fatal(err)
	}
	if _, me := call(http.MethodGet, nil); name(me) != renamed {
		t.Fatalf("depois de renomear no Authentik: %q", name(me))
	}

	if code, _ := call(http.MethodPut, "não é objeto"); code != http.StatusBadRequest {
		t.Fatalf("corpo inválido: %d", code)
	}
}
