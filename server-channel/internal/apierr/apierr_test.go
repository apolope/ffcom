package apierr

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Os mesmos padrões de scripts/check-locales.mjs (ERROR_CODE_PATTERN e
// ERROR_CALL_PATTERN): callPattern acha toda chamada que leva código, e
// codePattern, a partir do mesmo ponto, o código literal dela.
var (
	callPattern = regexp.MustCompile(`\bapierr\.(?:Write|WriteParams|New|NewParams)\(`)
	codePattern = regexp.MustCompile(`^apierr\.(?:Write|WriteParams|New|NewParams)\((?:\s*[\w.]+\s*,)*\s*"([a-z][a-z0-9_]*\.[a-z0-9_.]+)"`)
)

// localesFile fica na raiz do repositório, fora do módulo (por isso não há
// go:embed): server-channel/internal/apierr -> ../../../locales.
const localesFile = "../../../locales/pt-BR.json"

// moduleRoot é a raiz do módulo server-channel, varrida atrás dos códigos.
const moduleRoot = "../.."

// TestErrorCodesHaveTranslation falha se algum código usado no módulo não
// tiver chave errors.<code> em locales/pt-BR.json, ou se alguma chamada não
// tiver o código como string literal (o regex não o acharia).
func TestErrorCodesHaveTranslation(t *testing.T) {
	keys := loadErrorKeys(t)

	found := 0
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != moduleRoot && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		for _, loc := range callPattern.FindAllStringIndex(text, -1) {
			line := strings.Count(text[:loc[0]], "\n") + 1
			m := codePattern.FindStringSubmatch(text[loc[0]:])
			if m == nil {
				t.Errorf("%s:%d: chamada sem código literal no formato area.motivo", path, line)
				continue
			}
			found++
			if !keys["errors."+m[1]] {
				t.Errorf("%s:%d: código %q sem \"errors.%s\" em %s", path, line, m[1], m[1], localesFile)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("nenhum código de erro encontrado; o padrão ainda casa com as chamadas?")
	}
}

func loadErrorKeys(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(localesFile)
	if err != nil {
		t.Fatalf("ler %s: %v", localesFile, err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("%s: %v", localesFile, err)
	}
	keys := map[string]bool{}
	var walk func(prefix string, node map[string]any)
	walk = func(prefix string, node map[string]any) {
		for k, v := range node {
			switch v := v.(type) {
			case map[string]any:
				walk(prefix+k+".", v)
			case string:
				keys[prefix+k] = true
			}
		}
	}
	walk("", tree)
	return keys
}

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Length", "99")
	WriteParams(rec, http.StatusBadRequest, "ideas.text_length", "o texto precisa ter entre 10 e 1000 caracteres", Params{"min": 10, "max": 1000})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type %q", ct)
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Fatal("Content-Length anterior não foi descartado")
	}
	var out struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Params  map[string]any `json:"params"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != "ideas.text_length" || out.Message == "" || out.Params["max"] != float64(1000) {
		t.Fatalf("corpo errado: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	Write(rec, http.StatusNotFound, "channels.not_found", "canal não encontrado")
	if strings.Contains(rec.Body.String(), "params") {
		t.Fatalf("params vazio não deveria aparecer: %s", rec.Body.String())
	}
}
