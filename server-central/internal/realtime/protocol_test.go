package realtime

import (
	"encoding/json"
	"testing"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
)

// O frame "error" leva code/message/params como os erros HTTP e repete o
// texto em "error" para os clients anteriores ao code.
func TestEncodeError(t *testing.T) {
	raw, err := EncodeError(apierr.New("dm.send_friends_only", "só é possível enviar DMs para amigos"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["type"] != "error" || out["code"] != "dm.send_friends_only" || out["message"] != out["error"] || out["params"] != nil {
		t.Fatalf("frame errado: %s", raw)
	}

	if _, prob := DecodeIncomingDM([]byte(`{"type":"x"}`)); prob == nil || prob.Code != "realtime.frame_type_unknown" || prob.Params["type"] != "x" {
		t.Fatalf("DecodeIncomingDM com tipo desconhecido: %+v", prob)
	}
	if _, prob := DecodeIncomingPresenceIdle([]byte(`{"idle":"sim"}`)); prob == nil || prob.Code != "realtime.payload_invalid" {
		t.Fatalf("DecodeIncomingPresenceIdle com payload inválido: %+v", prob)
	}
}
