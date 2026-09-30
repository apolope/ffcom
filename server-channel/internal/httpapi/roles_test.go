package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// Testes de integração da política de roles e permissões (ver
// docs/architecture.md, "Decisão: ManageRoles não concede permissões além
// das próprias", e docs/permissions.md). Usam openTestStore
// (channels_admin_test.go), então precisam de FFCOM_TEST_DATABASE_URL.
//
// Os bits usados como "o que o moderador não tem" (Administrator,
// BanMembers, ManageInvites) ficam fora da role default de propósito: a
// @everyone é compartilhada pelo banco inteiro e começa com
// ViewChannels|SendMessages|Voice.

type roleFixture struct {
	t   *testing.T
	db  *store.Store
	ctx context.Context
}

func newRoleFixture(t *testing.T) roleFixture {
	return roleFixture{t: t, db: openTestStore(t), ctx: context.Background()}
}

func (f roleFixture) member(subject string) store.Member {
	f.t.Helper()
	m, err := f.db.Members.GetOrCreateByOIDCSubject(f.ctx, subject+"-"+f.t.Name())
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

// owner cria um dono de verdade no banco (is_owner), e não só no struct em
// memória, porque kick/ban relê o alvo do banco. O sufixo de tempo evita
// colidir com uma execução anterior no mesmo banco (CreateFounder não tem
// ON CONFLICT).
func (f roleFixture) owner() store.Member {
	f.t.Helper()
	m, err := f.db.Members.CreateFounder(f.ctx, fmt.Sprintf("owner-%s-%d", f.t.Name(), time.Now().UnixNano()))
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

func (f roleFixture) role(name string, bits int64) store.Role {
	f.t.Helper()
	r, err := f.db.Roles.Create(f.ctx, name+"-"+f.t.Name(), nil, bits, 1)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { f.db.Roles.Delete(context.Background(), r.ID) })
	return r
}

func (f roleFixture) assign(m store.Member, r store.Role) {
	f.t.Helper()
	if err := f.db.Roles.AssignToMember(f.ctx, m.ID, r.ID); err != nil {
		f.t.Fatal(err)
	}
}

func (f roleFixture) base(m store.Member) int64 {
	f.t.Helper()
	base, _, err := memberBasePermission(f.ctx, f.db.Roles, m)
	if err != nil {
		f.t.Fatal(err)
	}
	return base
}

func (f roleFixture) roleByID(id string) store.Role {
	f.t.Helper()
	r, err := findRole(f.ctx, f.db.Roles, id)
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f roleFixture) defaultRole() store.Role {
	f.t.Helper()
	r, err := f.db.Roles.GetDefault(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f roleFixture) channel(name string) store.Channel {
	f.t.Helper()
	c, err := f.db.Channels.Create(f.ctx, nil, name+"-"+f.t.Name(), store.ChannelText, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { f.db.Channels.Delete(context.Background(), c.ID) })
	return c
}

// do chama o handler como member, com os path values dados em pares
// ("id", x, "roleId", y...).
func (f roleFixture) do(h http.Handler, member store.Member, method string, body any, path ...string) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, "/", &buf)
	for i := 0; i+1 < len(path); i += 2 {
		req.SetPathValue(path[i], path[i+1])
	}
	req = req.WithContext(auth.WithMember(req.Context(), member))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func expectStatus(t *testing.T, what string, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("%s: status %d, esperado %d (%s)", what, rec.Code, want, bytes.TrimSpace(rec.Body.Bytes()))
	}
}

// Sem ManageRoles, nenhuma rota de escrita de role ou overwrite passa, e a
// listagem de roles continua aberta a qualquer membro.
func TestRoleRoutesRequireManageRoles(t *testing.T) {
	f := newRoleFixture(t)
	plain := f.member("plain")
	// KickMembers|BanMembers não incluem ManageRoles: moderar membro não
	// dá gerência de role.
	f.assign(plain, f.role("moderacao", permissions.KickMembers|permissions.BanMembers))
	other := f.role("outra", permissions.Voice)
	ch := f.channel("canal")

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"criar role":        f.do(handleCreateRole(f.db.Roles), plain, "POST", roleRequest{Name: "x"}),
		"editar role":       f.do(handleUpdateRole(f.db.Roles), plain, "PATCH", roleRequest{Name: "x"}, "id", other.ID),
		"apagar role":       f.do(handleDeleteRole(f.db.Roles), plain, "DELETE", nil, "id", other.ID),
		"atribuir role":     f.do(handleAssignRole(f.db.Members, f.db.Roles), plain, "POST", nil, "memberId", plain.ID, "roleId", other.ID),
		"desatribuir role":  f.do(handleRemoveRole(f.db.Roles), plain, "DELETE", nil, "memberId", plain.ID, "roleId", other.ID),
		"listar overwrites": f.do(handleListChannelOverwrites(f.db.Channels, f.db.Roles, f.db.ChannelOverwrites), plain, "GET", nil, "id", ch.ID),
		"gravar overwrite":  f.do(handleSetChannelOverwrite(f.db.Channels, f.db.Roles, f.db.ChannelOverwrites), plain, "PUT", setOverwriteRequest{Deny: permissions.SendMessages}, "id", ch.ID, "roleId", other.ID),
		"apagar overwrite":  f.do(handleDeleteChannelOverwrite(f.db.Roles, f.db.ChannelOverwrites), plain, "DELETE", nil, "id", ch.ID, "roleId", other.ID),
	} {
		expectStatus(t, name+" sem ManageRoles", rec, http.StatusForbidden)
	}

	// Nada mudou de fato.
	if got := f.roleByID(other.ID); got.Name != other.Name || got.Permissions != other.Permissions {
		t.Errorf("role alterada por quem não tem ManageRoles: %+v", got)
	}
	if rows, _ := f.db.ChannelOverwrites.ListForChannel(f.ctx, ch.ID); len(rows) != 0 {
		t.Errorf("overwrite gravado por quem não tem ManageRoles: %+v", rows)
	}

	expectStatus(t, "listar roles como membro comum", f.do(handleListRoles(f.db.Roles), plain, "GET", nil), http.StatusOK)
}

// O caminho de auto-escalonamento que a revisão de segurança fechou: quem
// tem ManageRoles sem Administrator não consegue, por nenhuma rota, chegar a
// um bit que não tinha, nem para si nem para outra pessoa.
func TestManageRolesCannotEscalate(t *testing.T) {
	f := newRoleFixture(t)
	mod := f.member("mod")
	modRole := f.role("mod", permissions.ManageRoles|permissions.KickMembers)
	f.assign(mod, modRole)
	accomplice := f.member("cumplice")
	modBase := f.base(mod)

	createRole := handleCreateRole(f.db.Roles)
	updateRole := handleUpdateRole(f.db.Roles)
	assignRole := handleAssignRole(f.db.Members, f.db.Roles)

	// 1. Criar role com bit que não tem.
	for name, bits := range map[string]int64{
		"Administrator":             permissions.Administrator,
		"BanMembers":                permissions.BanMembers,
		"bit que tem + bit que não": permissions.KickMembers | permissions.BanMembers,
		"todos os bits (-1)":        permissions.Owner,
	} {
		expectStatus(t, "criar role com "+name, f.do(createRole, mod, "POST", roleRequest{Name: "escalar-" + t.Name(), Permissions: bits}), http.StatusForbidden)
	}
	for _, r := range mustListRoles(t, f) {
		if r.Name == "escalar-"+t.Name() {
			t.Fatalf("role criada apesar do 403: %+v", r)
		}
	}

	// 2. Editar a própria role, ou a @everyone, para ganhar um bit.
	expectStatus(t, "editar a própria role adicionando Administrator",
		f.do(updateRole, mod, "PATCH", roleRequest{Name: modRole.Name, Permissions: modRole.Permissions | permissions.Administrator}, "id", modRole.ID),
		http.StatusForbidden)
	def := f.defaultRole()
	expectStatus(t, "editar a @everyone adicionando BanMembers",
		f.do(updateRole, mod, "PATCH", roleRequest{Name: def.Name, Color: def.Color, Permissions: def.Permissions | permissions.BanMembers, Position: def.Position}, "id", def.ID),
		http.StatusForbidden)
	if got := f.roleByID(modRole.ID).Permissions; got != modRole.Permissions {
		t.Errorf("role do moderador mudou para %d apesar do 403", got)
	}
	if got := f.defaultRole().Permissions; got != def.Permissions {
		t.Errorf("@everyone mudou para %d apesar do 403", got)
	}

	// 3. Atribuir uma role já existente, criada pelo dono, com bits a mais.
	adminRole := f.role("admin", permissions.Administrator)
	banRole := f.role("ban", permissions.BanMembers)
	for _, target := range []store.Member{mod, accomplice} {
		expectStatus(t, "atribuir role Administrator a "+target.OIDCSubject,
			f.do(assignRole, mod, "POST", nil, "memberId", target.ID, "roleId", adminRole.ID), http.StatusForbidden)
		expectStatus(t, "atribuir role BanMembers a "+target.OIDCSubject,
			f.do(assignRole, mod, "POST", nil, "memberId", target.ID, "roleId", banRole.ID), http.StatusForbidden)
	}
	if got := f.base(mod); got != modBase {
		t.Errorf("permissão base do moderador mudou de %d para %d", modBase, got)
	}
	if got := f.base(accomplice); permissions.Has(got, permissions.BanMembers) {
		t.Errorf("cúmplice ganhou BanMembers (base %d)", got)
	}

	// 4. Overwrite de canal liberando (allow) o que não tem. Deny é livre:
	// negar não concede nada.
	ch := f.channel("canal")
	setOverwrite := handleSetChannelOverwrite(f.db.Channels, f.db.Roles, f.db.ChannelOverwrites)
	for name, allow := range map[string]int64{
		"Administrator": permissions.Administrator,
		"BanMembers":    permissions.BanMembers,
		"ManageInvites": permissions.ManageInvites,
	} {
		expectStatus(t, "overwrite allow "+name,
			f.do(setOverwrite, mod, "PUT", setOverwriteRequest{Allow: allow}, "id", ch.ID, "roleId", modRole.ID), http.StatusForbidden)
	}
	if rows, _ := f.db.ChannelOverwrites.ListForChannel(f.ctx, ch.ID); len(rows) != 0 {
		t.Errorf("overwrite gravado apesar do 403: %+v", rows)
	}
	expectStatus(t, "overwrite só com deny de bits que não tem",
		f.do(setOverwrite, mod, "PUT", setOverwriteRequest{Deny: permissions.Administrator | permissions.BanMembers}, "id", ch.ID, "roleId", modRole.ID), http.StatusOK)
	if got, err := channelPermission(f.ctx, f.db.Roles, f.db.ChannelOverwrites, mod, ch.ID); err != nil || permissions.Has(got, permissions.BanMembers) {
		t.Errorf("deny acabou concedendo algo: perm %d, err %v", got, err)
	}
}

// O que ManageRoles continua podendo fazer: repassar bits que já tem, que é
// a razão de o bit existir separado de Administrator.
func TestManageRolesGrantsOwnBits(t *testing.T) {
	f := newRoleFixture(t)
	mod := f.member("mod")
	modRole := f.role("mod", permissions.ManageRoles|permissions.KickMembers)
	f.assign(mod, modRole)
	target := f.member("alvo")

	rec := f.do(handleCreateRole(f.db.Roles), mod, "POST", roleRequest{Name: "porteiro-" + t.Name(), Permissions: permissions.KickMembers | permissions.SendMessages})
	expectStatus(t, "criar role com bits que tem (inclusive da @everyone)", rec, http.StatusCreated)
	var created roleView
	json.NewDecoder(rec.Body).Decode(&created)
	t.Cleanup(func() { f.db.Roles.Delete(context.Background(), created.ID) })

	expectStatus(t, "atribuir essa role a outro membro",
		f.do(handleAssignRole(f.db.Members, f.db.Roles), mod, "POST", nil, "memberId", target.ID, "roleId", created.ID), http.StatusNoContent)
	if !permissions.Has(f.base(target), permissions.KickMembers) {
		t.Error("alvo não ganhou KickMembers depois da atribuição")
	}

	ch := f.channel("canal")
	expectStatus(t, "overwrite allow de bit que tem",
		f.do(handleSetChannelOverwrite(f.db.Channels, f.db.Roles, f.db.ChannelOverwrites), mod, "PUT", setOverwriteRequest{Allow: permissions.KickMembers}, "id", ch.ID, "roleId", created.ID),
		http.StatusOK)
}

// Teto por bits para remover e rebaixar: quem tem ManageRoles só edita,
// apaga, desatribui ou mexe em overwrite de role cujos bits ele tem. Fecha o
// caminho de um moderador tirar o Administrador de alguém. Ver
// docs/architecture.md, "Decisão: teto por bits para remover e rebaixar".
func TestManageRolesCannotDemote(t *testing.T) {
	f := newRoleFixture(t)
	mod := f.member("mod")
	modRole := f.role("mod", permissions.ManageRoles|permissions.KickMembers)
	f.assign(mod, modRole)
	admin := f.member("admin")
	adminRole := f.role("admin", permissions.Administrator)
	f.assign(admin, adminRole)
	banRole := f.role("ban", permissions.BanMembers)
	ch := f.channel("canal")
	if _, err := f.db.ChannelOverwrites.Set(f.ctx, ch.ID, banRole.ID, 0, permissions.Voice); err != nil {
		t.Fatal(err)
	}

	updateRole := handleUpdateRole(f.db.Roles)
	deleteRole := handleDeleteRole(f.db.Roles)
	removeRole := handleRemoveRole(f.db.Roles)
	setOverwrite := handleSetChannelOverwrite(f.db.Channels, f.db.Roles, f.db.ChannelOverwrites)
	deleteOverwrite := handleDeleteChannelOverwrite(f.db.Roles, f.db.ChannelOverwrites)

	for _, target := range []store.Role{adminRole, banRole} {
		expectStatus(t, "tirar os bits da role "+target.Name,
			f.do(updateRole, mod, "PATCH", roleRequest{Name: target.Name, Permissions: 0}, "id", target.ID), http.StatusForbidden)
		expectStatus(t, "renomear a role "+target.Name,
			f.do(updateRole, mod, "PATCH", roleRequest{Name: "x", Permissions: permissions.KickMembers}, "id", target.ID), http.StatusForbidden)
		expectStatus(t, "apagar a role "+target.Name, f.do(deleteRole, mod, "DELETE", nil, "id", target.ID), http.StatusForbidden)
		expectStatus(t, "negar num canal para a role "+target.Name,
			f.do(setOverwrite, mod, "PUT", setOverwriteRequest{Deny: permissions.SendMessages}, "id", ch.ID, "roleId", target.ID), http.StatusForbidden)
		if got := f.roleByID(target.ID); got.Name != target.Name || got.Permissions != target.Permissions {
			t.Errorf("role %s mudou apesar do 403: %+v", target.Name, got)
		}
	}
	expectStatus(t, "tirar a role de Administrador de alguém",
		f.do(removeRole, mod, "DELETE", nil, "memberId", admin.ID, "roleId", adminRole.ID), http.StatusForbidden)
	if !permissions.Has(f.base(admin), permissions.Administrator) {
		t.Error("o Administrador perdeu a role apesar do 403")
	}
	expectStatus(t, "apagar overwrite da role BanMembers",
		f.do(deleteOverwrite, mod, "DELETE", nil, "id", ch.ID, "roleId", banRole.ID), http.StatusForbidden)
	if rows, _ := f.db.ChannelOverwrites.ListForChannel(f.ctx, ch.ID); len(rows) != 1 {
		t.Errorf("overwrite da role BanMembers sumiu apesar do 403: %+v", rows)
	}

	// Dentro do teto, continua tudo liberado: a própria role, uma role com
	// bits que ele tem, e a @everyone.
	expectStatus(t, "renomear a própria role",
		f.do(updateRole, mod, "PATCH", roleRequest{Name: modRole.Name + "-2", Permissions: modRole.Permissions}, "id", modRole.ID), http.StatusOK)
	kickRole := f.role("kick", permissions.KickMembers)
	target := f.member("alvo")
	f.assign(target, kickRole)
	expectStatus(t, "negar num canal para a @everyone",
		f.do(setOverwrite, mod, "PUT", setOverwriteRequest{Deny: permissions.SendMessages}, "id", ch.ID, "roleId", f.defaultRole().ID), http.StatusOK)
	expectStatus(t, "tirar uma role dentro do teto",
		f.do(removeRole, mod, "DELETE", nil, "memberId", target.ID, "roleId", kickRole.ID), http.StatusNoContent)
	expectStatus(t, "apagar uma role dentro do teto", f.do(deleteRole, mod, "DELETE", nil, "id", kickRole.ID), http.StatusNoContent)

	// Administrator e dono não têm teto.
	owner := f.owner()
	expectStatus(t, "Administrator tira a role BanMembers de um canal",
		f.do(deleteOverwrite, admin, "DELETE", nil, "id", ch.ID, "roleId", banRole.ID), http.StatusNoContent)
	expectStatus(t, "dono apaga a role BanMembers", f.do(deleteRole, owner, "DELETE", nil, "id", banRole.ID), http.StatusNoContent)
}

// Administrator (sem ser dono) e o dono passam em Grants para qualquer bit.
func TestAdministratorAndOwnerGrantAnything(t *testing.T) {
	f := newRoleFixture(t)
	admin := f.member("admin")
	f.assign(admin, f.role("admin", permissions.Administrator))
	owner := f.owner()
	target := f.member("alvo")

	for who, requester := range map[string]store.Member{"Administrator": admin, "dono": owner} {
		rec := f.do(handleCreateRole(f.db.Roles), requester, "POST", roleRequest{Name: "banidor-" + who + "-" + t.Name(), Permissions: permissions.BanMembers | permissions.ManageInvites})
		expectStatus(t, who+" cria role com bits que a própria role não lista", rec, http.StatusCreated)
		var created roleView
		json.NewDecoder(rec.Body).Decode(&created)
		t.Cleanup(func() { f.db.Roles.Delete(context.Background(), created.ID) })

		expectStatus(t, who+" atribui essa role",
			f.do(handleAssignRole(f.db.Members, f.db.Roles), requester, "POST", nil, "memberId", target.ID, "roleId", created.ID), http.StatusNoContent)
	}
	if !permissions.Has(f.base(target), permissions.BanMembers) {
		t.Error("alvo não ganhou BanMembers")
	}
}

// A @everyone não pode ser apagada nem atribuída/desatribuída à mão.
func TestDefaultRoleGuards(t *testing.T) {
	f := newRoleFixture(t)
	owner := f.owner()
	target := f.member("alvo")
	def := f.defaultRole()

	expectStatus(t, "apagar a @everyone", f.do(handleDeleteRole(f.db.Roles), owner, "DELETE", nil, "id", def.ID), http.StatusBadRequest)
	expectStatus(t, "atribuir a @everyone", f.do(handleAssignRole(f.db.Members, f.db.Roles), owner, "POST", nil, "memberId", target.ID, "roleId", def.ID), http.StatusBadRequest)
	if _, err := f.db.Roles.GetDefault(f.ctx); err != nil {
		t.Fatalf("@everyone sumiu: %v", err)
	}
}

// Permissão efetiva num canal, com os overwrites vindos do banco: deny da
// @everyone tira o bit, allow de uma role do membro devolve (é o canal
// privado), e vence também o deny de outra role dele; overwrite de role que
// o membro não tem não conta, e Administrator e dono ignoram overwrites.
func TestChannelOverwritesEffectivePermission(t *testing.T) {
	f := newRoleFixture(t)
	ch := f.channel("privado")
	def := f.defaultRole()
	speaker := f.role("falante", 0)
	muted := f.role("silenciado", 0)
	unrelated := f.role("sem-relacao", 0)
	adminRole := f.role("admin", permissions.Administrator)

	plain := f.member("plain")
	withSpeaker := f.member("falante")
	f.assign(withSpeaker, speaker)
	speakerAndMuted := f.member("falante-silenciado")
	f.assign(speakerAndMuted, speaker)
	f.assign(speakerAndMuted, muted)
	onlyMuted := f.member("silenciado")
	f.assign(onlyMuted, muted)
	admin := f.member("admin")
	f.assign(admin, adminRole)
	owner := f.owner()

	set := func(roleID string, allow, deny int64) {
		t.Helper()
		if _, err := f.db.ChannelOverwrites.Set(f.ctx, ch.ID, roleID, allow, deny); err != nil {
			t.Fatal(err)
		}
	}
	set(def.ID, 0, permissions.SendMessages)
	set(speaker.ID, permissions.SendMessages, 0)
	set(muted.ID, 0, permissions.SendMessages)
	set(unrelated.ID, 0, permissions.ViewChannels)

	canSend := func(m store.Member) bool {
		t.Helper()
		p, err := channelPermission(f.ctx, f.db.Roles, f.db.ChannelOverwrites, m, ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		return permissions.Has(p, permissions.SendMessages)
	}
	if canSend(plain) {
		t.Error("deny da @everyone no canal não tirou SendMessages do membro comum")
	}
	if !canSend(withSpeaker) {
		t.Error("allow da role no canal não venceu o deny da @everyone")
	}
	if !canSend(speakerAndMuted) {
		t.Error("allow de uma role não venceu o deny de outra role do mesmo membro")
	}
	if canSend(onlyMuted) {
		t.Error("deny da role não tirou SendMessages")
	}
	if !canSend(admin) || !canSend(owner) {
		t.Error("Administrator/dono foram afetados por overwrite de canal")
	}
	if p, _ := channelPermission(f.ctx, f.db.Roles, f.db.ChannelOverwrites, plain, ch.ID); !permissions.Has(p, permissions.ViewChannels) {
		t.Error("deny de uma role que o membro não tem foi aplicado a ele")
	}

	// Fora desse canal, a base continua valendo.
	other := f.channel("aberto")
	if p, _ := channelPermission(f.ctx, f.db.Roles, f.db.ChannelOverwrites, plain, other.ID); !permissions.Has(p, permissions.SendMessages) {
		t.Error("overwrite vazou para outro canal")
	}
}

// Kick e ban: cada um exige o próprio bit, ninguém expulsa o dono nem a si
// mesmo, e o alvo não pode ter bit que quem expulsa não tem.
func TestModerationPermissions(t *testing.T) {
	f := newRoleFixture(t)
	kicker := f.member("kicker")
	f.assign(kicker, f.role("kicker", permissions.KickMembers))
	plain := f.member("plain")
	owner := f.owner()

	kick := handleKickMember(f.db.Members, f.db.Roles)
	ban := handleBanMember(f.db.Members, f.db.Roles, f.db.MemberBans)

	expectStatus(t, "kick sem KickMembers", f.do(kick, plain, "POST", nil, "memberId", kicker.ID), http.StatusForbidden)
	expectStatus(t, "ban só com KickMembers", f.do(ban, kicker, "POST", nil, "memberId", plain.ID), http.StatusForbidden)
	expectStatus(t, "kick no dono", f.do(kick, kicker, "POST", nil, "memberId", owner.ID), http.StatusForbidden)
	expectStatus(t, "kick em si mesmo", f.do(kick, kicker, "POST", nil, "memberId", kicker.ID), http.StatusBadRequest)
	admin := f.member("admin")
	f.assign(admin, f.role("admin", permissions.Administrator))
	banner := f.member("banner")
	f.assign(banner, f.role("banner", permissions.BanMembers))
	expectStatus(t, "kick num Administrador", f.do(kick, kicker, "POST", nil, "memberId", admin.ID), http.StatusForbidden)
	expectStatus(t, "kick em quem tem BanMembers", f.do(kick, kicker, "POST", nil, "memberId", banner.ID), http.StatusForbidden)
	expectStatus(t, "ban de Administrador por quem só tem BanMembers", f.do(ban, banner, "POST", nil, "memberId", admin.ID), http.StatusForbidden)
	for _, m := range []store.Member{plain, owner, kicker, admin, banner} {
		if got, err := f.db.Members.GetByID(f.ctx, m.ID); err != nil || got.RemovedAt != nil {
			t.Errorf("%s foi removido apesar da recusa (err %v)", m.OIDCSubject, err)
		}
	}

	expectStatus(t, "kick com KickMembers", f.do(kick, kicker, "POST", nil, "memberId", plain.ID), http.StatusNoContent)
	if got, _ := f.db.Members.GetByID(f.ctx, plain.ID); got.RemovedAt == nil {
		t.Error("kick respondeu 204 mas o membro continua ativo")
	}
}

func mustListRoles(t *testing.T, f roleFixture) []store.Role {
	t.Helper()
	rows, err := f.db.Roles.List(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
