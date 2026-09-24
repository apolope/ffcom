package store

import "testing"

func TestMemberDisplayName(t *testing.T) {
	nick, profile, empty := "Apolo", "Apolonio Serafim", ""
	const id = "dc5cf1f0-b183-4eee-8428-4a75401b78af"
	cases := []struct {
		name   string
		member Member
		want   string
	}{
		{"apelido vence", Member{ID: id, Nickname: &nick, ProfileName: &profile}, "Apolo"},
		{"sem apelido usa o perfil", Member{ID: id, ProfileName: &profile}, "Apolonio Serafim"},
		{"apelido vazio usa o perfil", Member{ID: id, Nickname: &empty, ProfileName: &profile}, "Apolonio Serafim"},
		{"sem nenhum usa o começo do id", Member{ID: id}, "dc5cf1f0"},
	}
	for _, c := range cases {
		if got := c.member.DisplayName(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
