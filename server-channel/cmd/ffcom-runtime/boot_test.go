package main

import (
	"testing"
	"time"
)

func TestDefaultTrack(t *testing.T) {
	cases := []struct{ seed, current, want string }{
		{"0.7.3", "0.8.0", "0.7"},
		{"", "0.8.1", "0.8"},
		{"", "", "latest"},
		{"lixo", "", "latest"},
	}
	for _, c := range cases {
		if got := defaultTrack(c.seed, c.current); got != c.want {
			t.Errorf("defaultTrack(%q, %q) = %q, quer %q", c.seed, c.current, got, c.want)
		}
	}
}

func TestChooseBoot(t *testing.T) {
	cases := []struct {
		name         string
		current      string
		currentOK    bool
		seed, track  string
		bad          map[string]bool
		want         string
		wantFromSeed bool
	}{
		{"nada", "", false, "", "0.7", nil, "", false},
		{"só semente", "", false, "0.7.0", "0.7", nil, "0.7.0", true},
		{"current corrompida usa semente", "0.7.5", false, "0.7.0", "0.7", nil, "0.7.0", true},
		{"current corrompida e semente em bad ainda usa semente", "0.7.5", false, "0.7.0", "0.7", map[string]bool{"0.7.0": true}, "0.7.0", true},
		{"só current", "0.7.1", true, "", "0.7", nil, "0.7.1", false},
		{"current mais nova que semente", "0.7.4", true, "0.7.0", "0.7", nil, "0.7.4", false},
		{"semente igual", "0.7.0", true, "0.7.0", "0.7", nil, "0.7.0", false},
		{"semente mais nova na trilha", "0.7.1", true, "0.7.3", "0.7", nil, "0.7.3", true},
		{"semente mais nova fora da trilha", "0.7.1", true, "0.8.0", "0.7", nil, "0.7.1", false},
		{"semente mais nova com latest", "0.7.1", true, "0.8.0", "latest", nil, "0.8.0", true},
		{"semente mais nova em bad", "0.7.1", true, "0.7.3", "0.7", map[string]bool{"0.7.3": true}, "0.7.1", false},
	}
	for _, c := range cases {
		got, fromSeed := chooseBoot(c.current, c.currentOK, c.seed, c.track, c.bad)
		if got != c.want || fromSeed != c.wantFromSeed {
			t.Errorf("%s: chooseBoot = %q, %v; quer %q, %v", c.name, got, fromSeed, c.want, c.wantFromSeed)
		}
	}
}

func TestShouldSwitch(t *testing.T) {
	cases := []struct {
		current, chosen, track string
		want                   bool
	}{
		{"", "0.7.0", "0.7", true},
		{"0.7.0", "0.7.1", "0.7", true},
		{"0.7.1", "0.7.1", "0.7", false},
		{"0.8.0", "0.7.5", "0.7", false},   // trilha mudou para trás: não rebaixa
		{"0.8.0", "0.7.5", "0.7.5", true},  // versão fixa: rebaixa
		{"0.7.5", "0.7.5", "0.7.5", false}, // já na fixa
	}
	for _, c := range cases {
		if got := shouldSwitch(c.current, c.chosen, c.track); got != c.want {
			t.Errorf("shouldSwitch(%q, %q, %q) = %v, quer %v", c.current, c.chosen, c.track, got, c.want)
		}
	}
}

func TestCrashPolicy(t *testing.T) {
	p := crashPolicy{max: 5, stableAfter: time.Minute, base: time.Second, maxDelay: 30 * time.Second}
	for i, w := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second} {
		d, giveUp := p.record(time.Second)
		if giveUp || d != w {
			t.Fatalf("falha %d: delay %s giveUp %v, quer %s", i+1, d, giveUp, w)
		}
	}
	if _, giveUp := p.record(time.Second); !giveUp {
		t.Error("5ª falha seguida deveria desistir")
	}
	// Um filho que ficou no ar zera a contagem.
	p.reset()
	p.record(time.Second)
	if d, giveUp := p.record(2 * time.Minute); giveUp || d != time.Second {
		t.Errorf("depois de uptime longo: delay %s giveUp %v", d, giveUp)
	}
	// Teto do atraso.
	q := crashPolicy{max: 100, stableAfter: time.Minute, base: time.Second, maxDelay: 30 * time.Second}
	var d time.Duration
	for range 10 {
		d, _ = q.record(0)
	}
	if d != 30*time.Second {
		t.Errorf("atraso sem teto: %s", d)
	}
}
