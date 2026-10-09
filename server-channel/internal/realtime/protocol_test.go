package realtime

import (
	"encoding/json"
	"testing"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
)

// O frame "error" leva code/message/params como os erros HTTP e repete o
// texto em "error" para os clients anteriores ao code.
func TestEncodeError(t *testing.T) {
	raw, err := encodeError(apierr.NewParams("messages.content_too_long", "conteúdo excede o limite de 4000 caracteres", apierr.Params{"max": 4000}))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	params, _ := out["params"].(map[string]any)
	if out["type"] != "error" || out["code"] != "messages.content_too_long" || out["message"] != out["error"] || params["max"] != float64(4000) {
		t.Fatalf("frame errado: %s", raw)
	}

	if _, prob := FrameType([]byte("{")); prob == nil || prob.Code != "realtime.frame_invalid" {
		t.Fatalf("FrameType com JSON inválido: %+v", prob)
	}
	if _, prob := DecodeIncoming([]byte(`{"type":"x"}`)); prob == nil || prob.Code != "realtime.frame_type_unknown" || prob.Params["type"] != "x" {
		t.Fatalf("DecodeIncoming com tipo desconhecido: %+v", prob)
	}
}

func TestDecodeChannelViewing(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"type":"channel.viewing","active":true}`:  true,
		`{"type":"channel.viewing","active":false}`: false,
	} {
		got, prob := DecodeChannelViewing([]byte(raw))
		if prob != nil || got != want {
			t.Fatalf("DecodeChannelViewing(%s) = %v, %+v", raw, got, prob)
		}
	}
	for _, raw := range []string{`{"type":"channel.viewing"}`, `{"type":"channel.viewing","active":"sim"}`} {
		if _, prob := DecodeChannelViewing([]byte(raw)); prob == nil || prob.Code != "realtime.payload_invalid" {
			t.Fatalf("DecodeChannelViewing(%s): %+v", raw, prob)
		}
	}
}
