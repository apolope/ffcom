package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
)

func TestPatchMeLanguage(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.Handle("GET /api/me", handleMe(db))
	mux.Handle("PATCH /api/me", handlePatchMe(db))

	subject := fmt.Sprintf("lang-%s-%d", t.Name(), time.Now().UnixNano())
	if _, err := db.Accounts.GetOrCreateBySubject(ctx, subject, nil); err != nil {
		t.Fatal(err)
	}

	// Como o auth.Middleware, cada requisição relê a conta do banco.
	call := func(method, body string) (int, map[string]any) {
		t.Helper()
		account, err := db.Accounts.GetOrCreateBySubject(ctx, subject, nil)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/api/me", bytes.NewReader([]byte(body)))
		req = req.WithContext(auth.WithAccount(req.Context(), account))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("resposta não é JSON: %s", rec.Body.String())
		}
		return rec.Code, out
	}

	// Sem escolha: "language" vem null, não ausente.
	code, me := call(http.MethodGet, "")
	if lang, present := me["language"]; code != http.StatusOK || !present || lang != nil {
		t.Fatalf("inicial: %d %v", code, me)
	}

	if code, me := call(http.MethodPatch, `{"language":"en"}`); code != http.StatusOK || me["language"] != "en" {
		t.Fatalf("definir en: %d %v", code, me)
	}
	if _, me := call(http.MethodGet, ""); me["language"] != "en" {
		t.Fatalf("GET depois de definir: %v", me["language"])
	}

	// Corpo sem "language" não mexe na escolha.
	if code, me := call(http.MethodPatch, `{}`); code != http.StatusOK || me["language"] != "en" {
		t.Fatalf("PATCH vazio: %d %v", code, me)
	}

	for _, bad := range []string{`{"language":"fr"}`, `{"language":"pt"}`, `{"language":""}`} {
		code, body := call(http.MethodPatch, bad)
		if code != http.StatusBadRequest || body["code"] != "profile.language_unsupported" {
			t.Fatalf("%s: %d %v", bad, code, body)
		}
	}
	for _, bad := range []string{`{"language":1}`, `"en"`, `null`, `não é json`} {
		code, body := call(http.MethodPatch, bad)
		if code != http.StatusBadRequest || body["code"] != "common.invalid_body" {
			t.Fatalf("%s: %d %v", bad, code, body)
		}
	}
	if _, me := call(http.MethodGet, ""); me["language"] != "en" {
		t.Fatalf("pedido inválido mudou o idioma: %v", me["language"])
	}

	// null apaga a escolha.
	if code, me := call(http.MethodPatch, `{"language":null}`); code != http.StatusOK || me["language"] != nil {
		t.Fatalf("apagar: %d %v", code, me)
	}
	if code, me := call(http.MethodPatch, `{"language":"pt-BR"}`); code != http.StatusOK || me["language"] != "pt-BR" {
		t.Fatalf("definir pt-BR: %d %v", code, me)
	}
}
