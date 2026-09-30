package main

import (
	"reflect"
	"testing"
)

func TestParseAllowedOriginsAlwaysIncludesDesktopApp(t *testing.T) {
	cases := map[string][]string{
		"":                                      {"app://ffcom"},
		"https://a.example, ,https://b.example": {"app://ffcom", "https://a.example", "https://b.example"},
		"https://a.example,app://ffcom":         {"app://ffcom", "https://a.example"},
	}
	for raw, want := range cases {
		if got := parseAllowedOrigins(raw); !reflect.DeepEqual(got, want) {
			t.Errorf("parseAllowedOrigins(%q) = %v, want %v", raw, got, want)
		}
	}
}
