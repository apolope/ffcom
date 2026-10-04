// Seção "Ideias": sugestões de melhoria com login no Authentik, varinha do
// Claude (via server-central e a3s-claude-relay), votos e moderação. As
// regras (uma ideia por dia, usos da varinha, penalidade, quem modera)
// vivem no server-central; aqui só a interface. Ver docs/architecture.md,
// "Decisão: sugestões de melhoria com varinha do Claude". Textos em
// site.ideas.* e erros do server-central em errors.* do arquivo de idioma
// (i18n.js); mensagens e avisos guardam a chave ou o erro, não o texto, e
// são traduzidos a cada render, então acompanham a troca de idioma.
(() => {
  const secao = document.getElementById('ideias');
  const i18n = window.ffcomI18n;
  if (!secao || !window.oidc || !i18n) return;
  const { t } = i18n;

  const CENTRAL = secao.dataset.central;
  const RASCUNHO = 'ffcom-ideia-rascunho';
  const MAX = 1000;
  const MIN = 10;

  const AUTHENTIK = 'https://authentik.abs.a3sitsolutions.com.br/application/o';
  const um = new window.oidc.UserManager({
    authority: `${AUTHENTIK}/ffcom/`,
    // Endpoints fixos em vez do documento de descoberta: o .well-known do
    // Authentik não devolvia Access-Control-Allow-Origin para esta origem
    // (só o endpoint de token), e o login quebrava antes do redirect. Com
    // metadata o navegador não busca a descoberta; os valores são os do
    // .well-known do provider ffcom e só mudam se o provider mudar de slug.
    metadata: {
      issuer: `${AUTHENTIK}/ffcom/`,
      authorization_endpoint: `${AUTHENTIK}/authorize/`,
      token_endpoint: `${AUTHENTIK}/token/`,
      userinfo_endpoint: `${AUTHENTIK}/userinfo/`,
      end_session_endpoint: `${AUTHENTIK}/ffcom/end-session/`,
      jwks_uri: `${AUTHENTIK}/ffcom/jwks/`,
      revocation_endpoint: `${AUTHENTIK}/revoke/`,
    },
    client_id: 'ffcom',
    redirect_uri: `${location.origin}/`,
    post_logout_redirect_uri: `${location.origin}/`,
    response_type: 'code',
    // ffcom-groups traz os grupos ffcom-* (quem é admin); o app não pede.
    scope: 'openid profile email offline_access ffcom-groups',
    automaticSilentRenew: true,
    userStore: new window.oidc.WebStorageStateStore({ store: window.localStorage }),
  });

  const el = {
    composer: document.getElementById('ideia-composer'),
    abas: document.getElementById('ideias-abas'),
    lista: document.getElementById('ideias-lista'),
    aviso: document.getElementById('ideias-aviso'),
  };

  const estado = {
    user: null,
    me: null,
    aba: 'ranking',
    ideias: [],
    varinha: null, // { id, status }
    parecidas: [],
    anterior: null, // texto antes da varinha, para desfazer
    mensagem: null, // { tipo: 'ok'|'erro'|'info', chave, params } ou { tipo, erro }
    aviso: null, // acima da lista: { chave, params } ou { erro }
    enviada: null, // { id, texto } da última sugestão enviada, até a checagem terminar
  };

  // ---------- utilidades

  const h = (tag, attrs = {}, ...filhos) => {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (v === false || v == null) continue;
      if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
      else if (k === 'class') n.className = v;
      else if (v === true) n.setAttribute(k, '');
      else n.setAttribute(k, v);
    }
    for (const f of filhos.flat()) if (f != null && f !== false) n.append(f);
    return n;
  };

  // replaceChildren converte null em texto "null"; esta versão pula os vazios.
  const trocar = (no, ...filhos) => no.replaceChildren(...filhos.flat().filter((f) => f != null && f !== false));

  const svg = (path, cls = 'ico') => {
    const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    s.setAttribute('viewBox', '0 0 24 24');
    s.setAttribute('class', cls);
    s.setAttribute('aria-hidden', 'true');
    s.innerHTML = path;
    return s;
  };
  const ICONES = {
    varinha: '<path d="M4 20 14.5 9.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M15 3v3M15 12v3M9.5 7.5h2M18.5 7.5h2M11.3 4.3l1.4 1.4M17.3 10.3l1.4 1.4M17.3 4.7l1.4-1.4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>',
    like: '<path d="M7 11v9H4v-9zM7 11l4-8c1.7 0 3 1.3 3 3v3h5.2a2 2 0 0 1 2 2.3l-1.2 7A2 2 0 0 1 18 20H7" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/>',
    dislike: '<path d="M17 13V4h3v9zM17 13l-4 8c-1.7 0-3-1.3-3-3v-3H4.8a2 2 0 0 1-2-2.3l1.2-7A2 2 0 0 1 6 4h11" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/>',
  };

  const fmtData = (d) => new Intl.DateTimeFormat(i18n.lang, { day: 'numeric', month: 'short' }).format(d);
  const primeiroNome = (user) => {
    const p = user?.profile ?? {};
    return String(p.given_name || p.name || p.preferred_username || '').trim().split(/\s+/)[0] || t('site.ideas.you');
  };

  // Os erros do server-central vêm em minúsculas e sem ponto final;
  // sozinhos na tela, ganham os dois.
  const frase = (texto) => (texto ? texto.charAt(0).toUpperCase() + texto.slice(1) + (/[.!?]$/.test(texto) ? '' : '.') : '');

  // Chave ou erro guardado em estado.mensagem/estado.aviso, no idioma atual.
  const textoDe = (m) => (m.erro ? m.erro.message : t(m.chave, m.params));

  const rascunho = {
    ler: () => { try { return sessionStorage.getItem(RASCUNHO) ?? ''; } catch { return ''; } },
    gravar: (t) => { try { sessionStorage.setItem(RASCUNHO, t); } catch { /* sem storage */ } },
    limpar: () => { try { sessionStorage.removeItem(RASCUNHO); } catch { /* sem storage */ } },
  };

  const espera = (ms) => new Promise((r) => setTimeout(r, ms));

  // ---------- API do server-central

  // Erro da API: {code, message, params?} (docs/protocol.md, "Erros da API
  // HTTP"). A message é traduzida a cada leitura pelo code, caindo na
  // message em português; `chave` é para erros do próprio site.
  class ErroApi extends Error {
    constructor(status, problema, chave) {
      super();
      this.status = status;
      this.problema = problema;
      this.chave = chave;
    }

    get message() {
      if (this.chave) return t(this.chave);
      return frase(i18n.errorText(this.problema)) || t('site.ideas.httpError', { status: this.status });
    }
  }

  const api = async (caminho, { method = 'GET', body, auth = true } = {}, tentouRenovar = false) => {
    const headers = {};
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (auth && estado.user) headers.Authorization = `Bearer ${estado.user.access_token}`;
    const r = await fetch(`${CENTRAL}${caminho}`, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
    if (r.status === 401 && auth && estado.user && !tentouRenovar) {
      try {
        estado.user = await um.signinSilent();
        return api(caminho, { method, body, auth }, true);
      } catch {
        await um.removeUser();
        estado.user = null;
        estado.me = null;
        render();
        throw new ErroApi(401, null, 'site.ideas.sessionExpired');
      }
    }
    const texto = await r.text();
    let dados = null;
    try { dados = texto ? JSON.parse(texto) : null; } catch { dados = null; }
    // `error` é o formato antigo; corpo que não é JSON (proxy na frente) vai
    // cru.
    if (!r.ok) {
      throw new ErroApi(r.status, dados && (dados.code || dados.message || dados.error)
        ? { code: dados.code, message: dados.message || dados.error, params: dados.params }
        : { message: texto.trim() });
    }
    return dados;
  };

  // ---------- login

  const entrar = () => {
    const campo = document.getElementById('ideia-texto');
    if (campo) rascunho.gravar(campo.value);
    // ui_locales: telas do Authentik no idioma do site (pt-BR ou en, que
    // ele aceita como estão).
    um.signinRedirect({ state: { voltar: '#ideias' }, extraQueryParams: { ui_locales: i18n.lang } });
  };

  const sair = async () => {
    await um.removeUser();
    estado.user = null;
    estado.me = null;
    estado.varinha = null;
    estado.parecidas = [];
    estado.mensagem = null;
    if (estado.aba === 'review') estado.aba = 'ranking';
    await carregarLista();
    render();
  };

  const voltarDoLogin = async () => {
    const q = new URLSearchParams(location.search);
    if (!q.has('code') || !q.has('state')) return;
    try {
      await um.signinRedirectCallback();
    } catch (e) {
      console.warn('Login não concluído:', e);
      estado.mensagem = { tipo: 'erro', chave: 'site.ideas.loginFailed' };
    }
    history.replaceState(null, '', `${location.pathname}#ideias`);
    secao.scrollIntoView();
  };

  // ---------- dados

  const carregarMe = async () => {
    if (!estado.user) return;
    try {
      estado.me = await api('/api/ideas/me');
    } catch (e) {
      if (e.status !== 401) estado.mensagem = { tipo: 'erro', erro: e };
    }
  };

  const avisar = (aviso) => {
    estado.aviso = aviso;
    el.aviso.textContent = aviso ? textoDe(aviso) : '';
  };

  const carregarLista = async () => {
    avisar({ chave: 'site.ideas.loadingIdeas' });
    try {
      estado.ideias = await api(`/api/ideas?view=${estado.aba}`, { auth: Boolean(estado.user) });
      avisar(null);
    } catch (e) {
      estado.ideias = [];
      avisar({ chave: e.status === 403 ? 'site.ideas.adminOnlyReview' : 'site.ideas.loadFailed' });
    }
    renderLista();
  };

  // ---------- varinha

  const usarVarinha = async () => {
    const campo = document.getElementById('ideia-texto');
    const texto = campo.value.trim();
    estado.mensagem = null;
    estado.parecidas = [];
    try {
      const r = await api('/api/ideas/assist', { method: 'POST', body: { text: texto } });
      estado.anterior = campo.value;
      estado.varinha = { id: r.id, status: r.status };
      estado.me.wandLeft = r.wandLeft;
      render();
      acompanharVarinha(r.id);
    } catch (e) {
      estado.mensagem = { tipo: 'erro', erro: e };
      await carregarMe();
      render();
    }
  };

  const acompanharVarinha = async (id) => {
    const limite = Date.now() + 7 * 60 * 1000;
    while (Date.now() < limite && estado.varinha?.id === id) {
      await espera(2500);
      if (document.hidden) continue;
      let r;
      try {
        r = await api(`/api/ideas/assist/${id}`);
      } catch {
        continue;
      }
      if (r.status === 'pending') continue;
      estado.varinha = null;
      estado.me.wandLeft = r.wandLeft;
      const campo = document.getElementById('ideia-texto');
      if (r.status === 'failed') {
        estado.anterior = null;
        estado.mensagem = { tipo: 'info', chave: 'site.ideas.wandFailed' };
      } else if (r.result.notSuggestion) {
        estado.anterior = null;
        estado.mensagem = { tipo: 'info', chave: 'site.ideas.notSuggestion', params: { hint: r.result.hint ?? '' } };
      } else if (r.result.offensive) {
        estado.anterior = null;
        if (campo) campo.value = '';
        rascunho.limpar();
        estado.mensagem = { tipo: 'erro', chave: 'site.ideas.offensive' };
      } else {
        if (campo) campo.value = r.result.text;
        rascunho.gravar(r.result.text);
        estado.parecidas = r.result.similar ?? [];
        estado.mensagem = { tipo: 'ok', chave: 'site.ideas.improved' };
      }
      await carregarMe();
      render();
      return;
    }
  };

  const desfazer = () => {
    const campo = document.getElementById('ideia-texto');
    if (campo && estado.anterior != null) {
      campo.value = estado.anterior;
      rascunho.gravar(campo.value);
    }
    estado.anterior = null;
    estado.mensagem = null;
    render();
  };

  // ---------- enviar

  const enviar = async (ev) => {
    ev.preventDefault();
    const campo = document.getElementById('ideia-texto');
    estado.mensagem = null;
    try {
      const criada = await api('/api/ideas', { method: 'POST', body: { text: campo.value.trim() } });
      estado.enviada = { id: criada.id, texto: campo.value };
      rascunho.limpar();
      estado.parecidas = [];
      estado.anterior = null;
      estado.mensagem = { tipo: 'info', chave: 'site.ideas.received' };
      await carregarMe();
      render();
      acompanharEnvio();
    } catch (e) {
      estado.mensagem = { tipo: 'erro', erro: e };
      render();
    }
  };

  const acompanharEnvio = async () => {
    const limite = Date.now() + 3 * 60 * 1000;
    while (Date.now() < limite) {
      await espera(3000);
      if (document.hidden) continue;
      await carregarMe();
      const descartada = estado.me?.lastDiscarded;
      if (descartada && estado.enviada && descartada.id === estado.enviada.id) {
        // Não era uma sugestão: o dia não foi gasto, o texto volta ao campo.
        rascunho.gravar(estado.enviada.texto);
        estado.enviada = null;
        estado.mensagem = { tipo: 'erro', chave: 'site.ideas.discarded', params: { feedback: descartada.feedback ?? '' } };
        render();
        return;
      }
      const s = estado.me?.todayIdea?.status;
      if (s && s !== 'checking') {
        estado.enviada = null;
        estado.mensagem = s === 'review'
          ? { tipo: 'info', chave: 'site.ideas.toReview' }
          : { tipo: 'ok', chave: 'site.ideas.published' };
        render();
        if (estado.aba === 'ranking') carregarLista();
        return;
      }
    }
    render();
  };

  // ---------- votos e moderação

  const votar = async (ideia, valor) => {
    if (!estado.user) return entrar();
    const novo = ideia.myVote === valor ? 0 : valor;
    try {
      const atualizada = await api(`/api/ideas/${ideia.id}/vote`, { method: 'PUT', body: { value: novo } });
      const trocar = (lista) => lista.map((i) => (i.id === atualizada.id ? atualizada : i));
      estado.ideias = trocar(estado.ideias);
      estado.parecidas = trocar(estado.parecidas);
      if (estado.aba === 'ranking') {
        estado.ideias.sort((a, b) => b.score - a.score || new Date(a.createdAt) - new Date(b.createdAt));
      }
      renderLista();
      renderComposer();
    } catch (e) {
      avisar({ erro: e });
    }
  };

  const excluir = async (ideia) => {
    try {
      await api(`/api/ideas/${ideia.id}`, { method: 'DELETE' });
      estado.ideias = estado.ideias.filter((i) => i.id !== ideia.id);
      estado.parecidas = estado.parecidas.filter((i) => i.id !== ideia.id);
      avisar(null);
      renderLista();
      renderComposer();
    } catch (e) {
      avisar({ erro: e });
    }
  };

  const moderar = async (ideia, status, versao) => {
    try {
      await api(`/api/ideas/${ideia.id}`, { method: 'PATCH', body: { status, implementedVersion: versao } });
      await carregarLista();
    } catch (e) {
      avisar({ erro: e });
    }
  };

  // ---------- renderização

  // Status e motivo de moderação vêm do server-central; sem tradução,
  // aparece o valor cru.
  const nomeStatus = (s) => (i18n.exists(`site.ideas.status.${s}`) ? t(`site.ideas.status.${s}`) : s);
  const nomeMotivo = (m) => (i18n.exists(`site.ideas.reviewReason.${m}`) ? t(`site.ideas.reviewReason.${m}`) : m);

  const botoesVoto = (ideia) => {
    const propria = ideia.mine;
    const votavel = ideia.status === 'open' || ideia.status === 'planned';
    if (!votavel) return null;
    const botao = (valor, icone, rotulo, qtd) => h('button', {
      type: 'button',
      class: `voto voto-${valor > 0 ? 'like' : 'dislike'}`,
      'aria-pressed': String(ideia.myVote === valor),
      'aria-label': `${rotulo} (${qtd})`,
      title: propria ? t('site.ideas.ownVote') : (estado.user ? rotulo : t('site.ideas.signInToVote')),
      disabled: propria,
      onclick: () => votar(ideia, valor),
    }, svg(ICONES[icone]), h('span', {}, String(qtd)));
    return h('div', { class: 'votos' },
      botao(1, 'like', t('site.ideas.like'), ideia.likes),
      botao(-1, 'dislike', t('site.ideas.dislike'), ideia.dislikes));
  };

  const acoesAdmin = (ideia) => {
    if (!estado.me?.isAdmin) return null;
    const acoes = h('div', { class: 'admin-acoes' });
    const botao = (rotulo, status, extra) => h('button', { type: 'button', class: 'btn-mini', onclick: () => moderar(ideia, status, extra?.()) }, rotulo);
    if (ideia.status === 'review' || ideia.status === 'checking') {
      acoes.append(botao(t('site.ideas.admin.approve'), 'open'), botao(t('site.ideas.admin.reject'), 'rejected'));
    } else if (ideia.status === 'open' || ideia.status === 'planned') {
      if (ideia.status === 'open') acoes.append(botao(t('site.ideas.admin.plan'), 'planned'));
      const idCampo = `versao-${ideia.id}`;
      const campo = h('input', { id: idCampo, class: 'campo-mini', placeholder: 'client v0.15.0', 'aria-label': t('site.ideas.admin.versionLabel') });
      acoes.append(campo, botao(t('site.ideas.admin.implemented'), 'implemented', () => campo.value.trim()), botao(t('site.ideas.admin.reject'), 'rejected'));
    } else if (ideia.status === 'implemented') {
      acoes.append(botao(t('site.ideas.admin.backToRanking'), 'open'));
    }
    // Excluir em dois cliques: o primeiro só arma o botão.
    const apagar = h('button', {
      type: 'button',
      class: 'btn-mini btn-perigo',
      title: t('site.ideas.admin.deleteTitle'),
      onclick: () => {
        if (apagar.dataset.armado) return excluir(ideia);
        apagar.dataset.armado = '1';
        apagar.textContent = t('site.ideas.admin.confirmDelete');
        setTimeout(() => {
          if (!apagar.isConnected) return;
          delete apagar.dataset.armado;
          apagar.textContent = t('site.ideas.admin.delete');
        }, 5000);
      },
    }, t('site.ideas.admin.delete'));
    acoes.append(apagar);
    return acoes;
  };

  const itemIdeia = (ideia, { compacto = false } = {}) => {
    const meta = h('p', { class: 'ideia-meta' }, t('site.ideas.byline', { author: ideia.author, date: fmtData(new Date(ideia.createdAt)) }));
    if (ideia.status === 'planned') meta.append(h('span', { class: 'etiqueta etiqueta-planejada' }, nomeStatus('planned')));
    if (ideia.status === 'implemented' && ideia.implementedVersion) {
      meta.append(h('a', { class: 'etiqueta etiqueta-implementada', href: '#versoes' }, ideia.implementedVersion));
    }
    if (ideia.mine) meta.append(h('span', { class: 'etiqueta' }, t('site.ideas.mine')));
    if (ideia.reviewReason && (ideia.status === 'review')) {
      meta.append(h('span', { class: 'etiqueta etiqueta-alerta' }, nomeMotivo(ideia.reviewReason)));
    }
    return h('li', { class: `ideia${compacto ? ' compacta' : ''}` },
      h('div', { class: 'placar', 'aria-label': t('site.ideas.scoreLabel', { count: ideia.score }) },
        h('span', { class: 'placar-num' }, String(ideia.score)),
        h('span', { class: 'placar-rot' }, t('site.ideas.points', { count: Math.abs(ideia.score) }))),
      h('div', { class: 'ideia-corpo' },
        ideia.title ? h('h3', {}, ideia.title) : null,
        h('p', { class: 'ideia-texto' }, ideia.body),
        meta,
        compacto ? null : acoesAdmin(ideia)),
      botoesVoto(ideia));
  };

  const VAZIO = {
    ranking: 'site.ideas.empty.ranking',
    implemented: 'site.ideas.empty.implemented',
    review: 'site.ideas.empty.review',
  };

  const renderLista = () => {
    el.lista.replaceChildren(...estado.ideias.map((i) => itemIdeia(i)));
    if (!estado.ideias.length && !estado.aviso) avisar({ chave: VAZIO[estado.aba] });
  };

  const renderAbas = () => {
    const abas = [['ranking', t('site.ideas.tabs.ranking')], ['implemented', t('site.ideas.tabs.implemented')]];
    if (estado.me?.isAdmin) abas.push(['review', t('site.ideas.tabs.review')]);
    el.abas.replaceChildren(...abas.map(([valor, rotulo]) => h('button', {
      type: 'button',
      class: 'filtro',
      'aria-pressed': String(estado.aba === valor),
      onclick: () => {
        if (estado.aba === valor) return;
        estado.aba = valor;
        renderAbas();
        carregarLista();
      },
    }, rotulo)));
  };

  const mensagem = () => (estado.mensagem
    ? h('p', { class: `ideia-msg ideia-msg-${estado.mensagem.tipo}`, role: estado.mensagem.tipo === 'erro' ? 'alert' : 'status' }, textoDe(estado.mensagem))
    : null);

  const renderComposer = () => {
    const c = el.composer;
    if (!estado.user) {
      trocar(c, 
        h('div', { class: 'composer-convite' },
          h('div', {},
            h('h3', {}, t('site.ideas.invite.title')),
            h('p', {}, t('site.ideas.invite.body'))),
          h('button', { type: 'button', class: 'btn', onclick: entrar }, t('site.ideas.invite.signIn'))),
        mensagem());
      return;
    }

    const topo = h('div', { class: 'composer-topo' },
      h('span', {}, t('site.ideas.hello', { name: primeiroNome(estado.user) })),
      h('button', { type: 'button', class: 'link-botao', onclick: sair }, t('site.ideas.signOut')));

    const me = estado.me;
    if (!me) {
      trocar(c, topo, h('p', { class: 'versoes-aviso' }, t('site.ideas.loading')), mensagem());
      return;
    }

    if (me.suggestedToday && me.todayIdea) {
      const ideia = me.todayIdea;
      trocar(c, topo,
        h('div', { class: 'ideia-do-dia' },
          h('p', { class: 'ideia-do-dia-rot' }, t('site.ideas.today', { status: nomeStatus(ideia.status) })),
          h('p', {}, ideia.body),
          h('p', { class: 'versoes-aviso' }, ideia.status === 'review'
            ? t('site.ideas.todayReview')
            : t('site.ideas.todayTomorrow'))),
        mensagem());
      return;
    }

    const trabalhando = Boolean(estado.varinha);
    const usados = me.wandLimit - me.wandLeft;
    const campo = h('textarea', {
      id: 'ideia-texto',
      rows: '5',
      maxlength: String(MAX),
      placeholder: t('site.ideas.placeholder'),
      readonly: trabalhando,
      'aria-describedby': 'ideia-contador',
    });
    const valorAtual = document.getElementById('ideia-texto')?.value;
    campo.value = valorAtual ?? rascunho.ler();
    const contador = h('span', { id: 'ideia-contador', class: 'contador' });
    const varinha = h('button', {
      type: 'button',
      class: `varinha${trabalhando ? ' trabalhando' : ''}`,
      onclick: usarVarinha,
      'aria-label': me.assistEnabled
        ? t('site.ideas.wandLabel', { used: usados, limit: me.wandLimit })
        : t('site.ideas.wandUnavailableLabel'),
    }, svg(ICONES.varinha), h('span', {}, trabalhando ? t('site.ideas.wandWorking') : t('site.ideas.wandButton')),
    h('span', { class: 'varinha-contador', title: t('site.ideas.wandCounterTitle', { used: usados, limit: me.wandLimit }) }, `${usados}/${me.wandLimit}`));
    const enviarBtn = h('button', { type: 'submit', class: 'btn' }, t('site.ideas.submit'));

    const atualizar = () => {
      const n = campo.value.trim().length;
      contador.textContent = `${campo.value.length}/${MAX}`;
      varinha.disabled = trabalhando || !me.assistEnabled || me.wandLeft <= 0 || n < MIN;
      enviarBtn.disabled = trabalhando || n < MIN;
    };
    campo.addEventListener('input', () => { rascunho.gravar(campo.value); atualizar(); });
    atualizar();

    const parecidas = estado.parecidas.length
      ? h('div', { class: 'parecidas' },
        h('p', { class: 'parecidas-rot' }, t('site.ideas.similar')),
        h('ol', { class: 'ideias-lista' }, estado.parecidas.map((i) => itemIdeia(i, { compacto: true }))))
      : null;

    trocar(c, topo,
      h('form', { class: 'composer-form', onsubmit: enviar },
        h('label', { for: 'ideia-texto', class: 'composer-rot' }, t('site.ideas.yourSuggestion')),
        campo,
        h('div', { class: 'composer-barra' },
          varinha,
          estado.anterior != null && !trabalhando ? h('button', { type: 'button', class: 'link-botao', onclick: desfazer }, t('site.ideas.undo')) : null,
          contador,
          enviarBtn),
        trabalhando ? h('p', { class: 'versoes-aviso' }, t('site.ideas.claudeReviewing')) : null,
        !me.assistEnabled ? h('p', { class: 'versoes-aviso' }, t('site.ideas.wandDisabled')) : null),
      mensagem(),
      parecidas);
  };

  const render = () => {
    renderComposer();
    renderAbas();
  };

  // ---------- início

  // Troca de idioma: redesenha tudo com os textos novos, sem buscar de novo.
  i18n.onChange(() => {
    render();
    renderLista();
    avisar(estado.aviso);
  });

  const iniciar = async () => {
    await i18n.ready;
    await voltarDoLogin();
    estado.user = await um.getUser();
    if (estado.user?.expired) {
      try { estado.user = await um.signinSilent(); } catch { await um.removeUser(); estado.user = null; }
    }
    await carregarMe();
    render();
    await carregarLista();
    if (estado.me?.pendingAssist) {
      estado.varinha = { id: estado.me.pendingAssist, status: 'pending' };
      render();
      acompanharVarinha(estado.me.pendingAssist);
    }
    if (estado.me?.todayIdea?.status === 'checking') {
      estado.enviada = { id: estado.me.todayIdea.id, texto: estado.me.todayIdea.body };
      acompanharEnvio();
    }
  };

  iniciar().catch((e) => {
    console.warn('Ideias indisponíveis:', e);
    avisar({ chave: 'site.ideas.loadFailed' });
  });
})();
