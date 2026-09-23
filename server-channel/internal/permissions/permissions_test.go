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
