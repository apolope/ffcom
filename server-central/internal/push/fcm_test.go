package push

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// FCM falso: um endpoint de token OAuth2 que confere a assinatura do JWT
// com a chave pública da conta de serviço, e o messages:send, que exige o
// token emitido e responde UNREGISTERED para o aparelho "morto".
func TestFCMSender(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	var tokenCalls atomic.Int32
	var lastBody map[string]any
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		r.ParseForm()
		parts := strings.Split(r.Form.Get("assertion"), ".")
		if len(parts) != 3 || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			http.Error(w, "asserção mal formada", http.StatusBadRequest)
			return
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
			http.Error(w, "assinatura inválida", http.StatusUnauthorized)
			return
		}
		claimsRaw, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claims map[string]any
		json.Unmarshal(claimsRaw, &claims)
		if claims["iss"] != "svc@proj.iam.gserviceaccount.com" || claims["scope"] != fcmScope || claims["aud"] != srv.URL+"/token" {
			http.Error(w, "claims erradas", http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"access_token":"tok-1","expires_in":3600,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("POST /v1/projects/proj/messages:send", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, "sem token", http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &lastBody)
		msg := lastBody["message"].(map[string]any)
		if msg["token"] == "dead" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"code":404,"status":"NOT_FOUND","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`))
			return
		}
		w.Write([]byte(`{"name":"projects/proj/messages/1"}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	saJSON, _ := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "proj",
		"client_email": "svc@proj.iam.gserviceaccount.com",
		"private_key":  pemKey,
		"token_uri":    srv.URL + "/token",
	})
	sender, err := NewFCMSender(string(saJSON))
	if err != nil {
		t.Fatal(err)
	}
	sender.baseURL = srv.URL

	ctx := context.Background()
	data := map[string]string{"v": "1", "type": TypeChannelMessage, "text": "oi"}
	if err := sender.Send(ctx, "alive", data); err != nil {
		t.Fatalf("send: %v", err)
	}
	msg := lastBody["message"].(map[string]any)
	if _, has := msg["notification"]; has {
		t.Error("mensagem não pode ter notification (só dados)")
	}
	if msg["data"].(map[string]any)["text"] != "oi" {
		t.Errorf("data: %+v", msg["data"])
	}
	if msg["android"].(map[string]any)["priority"] != "HIGH" {
		t.Errorf("android: %+v", msg["android"])
	}
	if err := sender.Send(ctx, "dead", data); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("token morto: %v, esperado ErrUnregistered", err)
	}
	if n := tokenCalls.Load(); n != 1 {
		t.Errorf("token OAuth2 pedido %d vezes, esperado 1 (cache)", n)
	}

	if _, err := NewFCMSender(`{"project_id":"p"}`); err == nil {
		t.Error("JSON sem chave deveria falhar")
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("  oi  ", 10); got != "oi" {
		t.Errorf("%q", got)
	}
	if got := Truncate("abcdef", 4); got != "abc…" {
		t.Errorf("%q", got)
	}
	if got := Truncate("ééééé", 3); got != "éé…" {
		t.Errorf("%q", got)
	}
}
