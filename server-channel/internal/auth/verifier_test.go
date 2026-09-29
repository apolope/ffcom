package auth

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func TestParseIssuerURLs(t *testing.T) {
	got := ParseIssuerURLs(" https://a/o/ffcom/ ,, https://b/o/ffcom/")
	want := []string{"https://a/o/ffcom/", "https://b/o/ffcom/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseIssuerURLs = %q, want %q", got, want)
	}
}

func TestUnverifiedIssuer(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://b/o/ffcom/","sub":"x"}`))
	got, err := unverifiedIssuer("e30." + payload + ".sig")
	if err != nil || got != "https://b/o/ffcom/" {
		t.Fatalf("unverifiedIssuer = %q, %v", got, err)
	}
	if _, err := unverifiedIssuer("nao-e-jwt"); err == nil {
		t.Fatal("token malformado deveria falhar")
	}
}
