package permissions

import "testing"

// TestHasAnyOfMask cobre o uso de Has com máscara de mais de um bit (POST
// /api/invites aceita CreateInvites ou ManageInvites): basta um deles.
func TestHasAnyOfMask(t *testing.T) {
	mask := CreateInvites | ManageInvites
	cases := []struct {
		name string
		base int64
		want bool
	}{
		{"só CreateInvites", CreateInvites, true},
		{"só ManageInvites", ManageInvites, true},
		{"nenhum dos dois", ViewChannels | SendMessages | ManageRoles, false},
		{"Administrator", Administrator, true},
		{"Owner", Owner, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Has(c.base, mask); got != c.want {
				t.Errorf("Has(%d, %d) = %v, want %v", c.base, mask, got, c.want)
			}
		})
	}
}

// TestGrants cobre o caso que motivou esta função: um membro com apenas
// ManageRoles (sem Administrator) não pode, via Grants, ser autorizado a
// conceder um bit que ele mesmo não possui — inclusive Administrator. Ver
// docs/architecture.md, "ManageRoles não concede permissões além das
// próprias".
func TestGrants(t *testing.T) {
	cases := []struct {
		name   string
		base   int64
		target int64
		want   bool
	}{
		{"sem bits, sem alvo", 0, 0, true},
		{"ManageRoles sozinho tentando conceder Administrator", ManageRoles, Administrator, false},
		{"ManageRoles sozinho tentando conceder um bit que não tem", ManageRoles, Voice, false},
		{"tem exatamente os bits do alvo", ManageRoles | Voice, Voice, true},
		{"alvo é subconjunto da base", ManageRoles | Voice | SendMessages, Voice, true},
		{"Administrator concede qualquer coisa", Administrator, Voice | ManageInvites, true},
		{"Owner (-1) concede qualquer coisa", Owner, Administrator | ManageRoles, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Grants(c.base, c.target); got != c.want {
				t.Errorf("Grants(%d, %d) = %v, want %v", c.base, c.target, got, c.want)
			}
		})
	}
}

// TestEffective cobre a precedência de overwrites no modelo do Discord: a
// @everyone primeiro, depois as outras roles do membro juntas, com allow
// vencendo deny. Ver docs/architecture.md, "Decisão: precedência de
// overwrites de canal no modelo do Discord".
func TestEffective(t *testing.T) {
	const everyone, mod, muted, other = "everyone", "mod", "muted", "other"
	base := ViewChannels | SendMessages | Voice
	cases := []struct {
		name       string
		base       int64
		roles      []string
		overwrites []Overwrite
		want       int64
	}{
		{"sem overwrite", base, []string{everyone}, nil, base},
		{"deny da @everyone tira o bit",
			base, []string{everyone},
			[]Overwrite{{RoleID: everyone, Deny: ViewChannels}},
			SendMessages | Voice},
		{"allow da @everyone dá um bit fora da base",
			base, []string{everyone},
			[]Overwrite{{RoleID: everyone, Allow: CreateInvites}},
			base | CreateInvites},
		{"canal privado: allow de role vence deny da @everyone",
			base, []string{mod, everyone},
			[]Overwrite{{RoleID: everyone, Deny: ViewChannels}, {RoleID: mod, Allow: ViewChannels}},
			base},
		{"ordem das linhas não importa",
			base, []string{mod, everyone},
			[]Overwrite{{RoleID: mod, Allow: ViewChannels}, {RoleID: everyone, Deny: ViewChannels}},
			base},
		{"allow de uma role vence deny de outra role do mesmo membro",
			base, []string{mod, muted, everyone},
			[]Overwrite{{RoleID: mod, Allow: SendMessages}, {RoleID: muted, Deny: SendMessages}},
			base},
		{"deny de role vence a base e o allow da @everyone",
			base, []string{muted, everyone},
			[]Overwrite{{RoleID: everyone, Allow: SendMessages}, {RoleID: muted, Deny: SendMessages}},
			ViewChannels | Voice},
		{"overwrite de role que o membro não tem não conta",
			base, []string{everyone},
			[]Overwrite{{RoleID: other, Deny: ViewChannels}, {RoleID: mod, Allow: ManageRoles}},
			base},
		{"Administrator ignora overwrites",
			Administrator, []string{everyone},
			[]Overwrite{{RoleID: everyone, Deny: ViewChannels}},
			Administrator},
		{"Owner ignora overwrites",
			Owner, nil,
			[]Overwrite{{RoleID: everyone, Deny: ViewChannels}},
			Owner},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Effective(c.base, everyone, c.roles, c.overwrites); got != c.want {
				t.Errorf("Effective = %d, want %d", got, c.want)
			}
		})
	}
}
