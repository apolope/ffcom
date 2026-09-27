// Seção "Ideias": sugestões de melhoria com login no Authentik, varinha do
// Claude (via server-central e a3s-claude-relay), votos e moderação. As
// regras (uma ideia por dia, usos da varinha, penalidade, quem modera)
// vivem no server-central; aqui só a interface. Ver docs/architecture.md,
// "Decisão: sugestões de melhoria com varinha do Claude".
(() => {
  const secao = document.getElementById('ideias');
  if (!secao || !window.oidc) return;

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
    mensagem: null, // { tipo: 'ok'|'erro'|'info', texto }
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

  const fmtData = new Intl.DateTimeFormat('pt-BR', { day: 'numeric', month: 'short' });
  const primeiroNome = (user) => {
    const p = user?.profile ?? {};
    return String(p.given_name || p.name || p.preferred_username || '').trim().split(/\s+/)[0] || 'você';
  };

  const rascunho = {
    ler: () => { try { return sessionStorage.getItem(RASCUNHO) ?? ''; } catch { return ''; } },
    gravar: (t) => { try { sessionStorage.setItem(RASCUNHO, t); } catch { /* sem storage */ } },
    limpar: () => { try { sessionStorage.removeItem(RASCUNHO); } catch { /* sem storage */ } },
  };

  const espera = (ms) => new Promise((r) => setTimeout(r, ms));

  // ---------- API do server-central

  class ErroApi extends Error {
    constructor(status, texto) { super(texto); this.status = status; }
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
        throw new ErroApi(401, 'Sua sessão expirou. Entre de novo.');
      }
    }
    const texto = await r.text();
    let dados = null;
    try { dados = texto ? JSON.parse(texto) : null; } catch { dados = null; }
    if (!r.ok) throw new ErroApi(r.status, (dados && dados.error) || texto.trim() || `Erro ${r.status}`);
    return dados;
  };

  // ---------- login

  const entrar = () => {
    const campo = document.getElementById('ideia-texto');
    if (campo) rascunho.gravar(campo.value);
    um.signinRedirect({ state: { voltar: '#ideias' } });
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
      estado.mensagem = { tipo: 'erro', texto: 'Não foi possível concluir o login. Confira se sua conta tem acesso ao FFCom.' };
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
      if (e.status !== 401) estado.mensagem = { tipo: 'erro', texto: e.message };
    }
  };

  const carregarLista = async () => {
    el.aviso.textContent = 'Carregando ideias…';
    try {
      estado.ideias = await api(`/api/ideas?view=${estado.aba}`, { auth: Boolean(estado.user) });
      el.aviso.textContent = '';
    } catch (e) {
      estado.ideias = [];
      el.aviso.textContent = e.status === 403 ? 'Só administradores veem a moderação.' : 'Não foi possível carregar as ideias agora.';
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
      estado.mensagem = { tipo: 'erro', texto: e.message };
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
        estado.mensagem = { tipo: 'info', texto: 'A varinha não conseguiu responder agora. Seu uso foi devolvido e o texto ficou como estava.' };
      } else if (r.result.notSuggestion) {
        estado.anterior = null;
        estado.mensagem = { tipo: 'info', texto: `Isso ainda não parece uma sugestão de melhoria. ${r.result.hint}` };
      } else if (r.result.offensive) {
        estado.anterior = null;
        if (campo) campo.value = '';
        rascunho.limpar();
        estado.mensagem = { tipo: 'erro', texto: 'Essa sugestão foi considerada ofensiva e descartada. Além do uso, ela custou usos extras da varinha de hoje.' };
      } else {
        if (campo) campo.value = r.result.text;
        rascunho.gravar(r.result.text);
        estado.parecidas = r.result.similar ?? [];
        estado.mensagem = { tipo: 'ok', texto: 'Texto melhorado. Revise e ajuste o que quiser antes de enviar.' };
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
      estado.mensagem = { tipo: 'info', texto: 'Recebemos sua sugestão. Ela passa por uma verificação rápida antes de aparecer.' };
      await carregarMe();
      render();
      acompanharEnvio();
    } catch (e) {
      estado.mensagem = { tipo: 'erro', texto: e.message };
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
        estado.mensagem = {
          tipo: 'erro',
          texto: `Não publicamos porque o texto não parece uma sugestão de melhoria. ${descartada.feedback ?? ''} Você pode reescrever e enviar de novo.`,
        };
        render();
        return;
      }
      const s = estado.me?.todayIdea?.status;
      if (s && s !== 'checking') {
        estado.enviada = null;
        estado.mensagem = s === 'review'
          ? { tipo: 'info', texto: 'Sua sugestão vai passar por um moderador antes de aparecer na lista.' }
          : { tipo: 'ok', texto: 'Sua sugestão foi publicada. Agora é torcer pelos votos.' };
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
      el.aviso.textContent = e.message;
    }
  };

  const excluir = async (ideia) => {
    try {
      await api(`/api/ideas/${ideia.id}`, { method: 'DELETE' });
      estado.ideias = estado.ideias.filter((i) => i.id !== ideia.id);
      estado.parecidas = estado.parecidas.filter((i) => i.id !== ideia.id);
      el.aviso.textContent = '';
      renderLista();
      renderComposer();
    } catch (e) {
      el.aviso.textContent = e.message;
    }
  };

  const moderar = async (ideia, status, versao) => {
    try {
      await api(`/api/ideas/${ideia.id}`, { method: 'PATCH', body: { status, implementedVersion: versao } });
      await carregarLista();
    } catch (e) {
      el.aviso.textContent = e.message;
    }
  };

  // ---------- renderização

  const STATUS = {
    checking: 'Em verificação',
    review: 'Em moderação',
    open: 'Publicada',
    planned: 'Planejada',
    implemented: 'Implementada',
    rejected: 'Recusada',
    discarded: 'Descartada',
  };
  const MOTIVO = {
    conteudo_ofensivo: 'a verificação apontou conteúdo ofensivo',
    verificacao_indisponivel: 'a verificação automática não pôde rodar',
  };

  const botoesVoto = (ideia) => {
    const propria = ideia.mine;
    const votavel = ideia.status === 'open' || ideia.status === 'planned';
    if (!votavel) return null;
    const botao = (valor, icone, rotulo, qtd) => h('button', {
      type: 'button',
      class: `voto voto-${valor > 0 ? 'like' : 'dislike'}`,
      'aria-pressed': String(ideia.myVote === valor),
      'aria-label': `${rotulo} (${qtd})`,
      title: propria ? 'Você não pode votar na própria ideia' : (estado.user ? rotulo : 'Entre para votar'),
      disabled: propria,
      onclick: () => votar(ideia, valor),
    }, svg(ICONES[icone]), h('span', {}, String(qtd)));
    return h('div', { class: 'votos' },
      botao(1, 'like', 'Gostei', ideia.likes),
      botao(-1, 'dislike', 'Não gostei', ideia.dislikes));
  };

  const acoesAdmin = (ideia) => {
    if (!estado.me?.isAdmin) return null;
    const acoes = h('div', { class: 'admin-acoes' });
    const botao = (rotulo, status, extra) => h('button', { type: 'button', class: 'btn-mini', onclick: () => moderar(ideia, status, extra?.()) }, rotulo);
    if (ideia.status === 'review' || ideia.status === 'checking') {
      acoes.append(botao('Aprovar', 'open'), botao('Recusar', 'rejected'));
    } else if (ideia.status === 'open' || ideia.status === 'planned') {
      if (ideia.status === 'open') acoes.append(botao('Planejar', 'planned'));
      const idCampo = `versao-${ideia.id}`;
      const campo = h('input', { id: idCampo, class: 'campo-mini', placeholder: 'client v0.15.0', 'aria-label': 'Versão em que foi implementada' });
      acoes.append(campo, botao('Implementada', 'implemented', () => campo.value.trim()), botao('Recusar', 'rejected'));
    } else if (ideia.status === 'implemented') {
      acoes.append(botao('Voltar ao ranking', 'open'));
    }
    // Excluir em dois cliques: o primeiro só arma o botão.
    const apagar = h('button', {
      type: 'button',
      class: 'btn-mini btn-perigo',
      title: 'Apaga a ideia e os votos de vez. Diferente de Recusar, devolve ao autor a sugestão do dia.',
      onclick: () => {
        if (apagar.dataset.armado) return excluir(ideia);
        apagar.dataset.armado = '1';
        apagar.textContent = 'Confirmar exclusão';
        setTimeout(() => {
          if (!apagar.isConnected) return;
          delete apagar.dataset.armado;
          apagar.textContent = 'Excluir';
        }, 5000);
      },
    }, 'Excluir');
    acoes.append(apagar);
    return acoes;
  };

  const itemIdeia = (ideia, { compacto = false } = {}) => {
    const meta = h('p', { class: 'ideia-meta' }, `por ${ideia.author} · ${fmtData.format(new Date(ideia.createdAt))}`);
    if (ideia.status === 'planned') meta.append(h('span', { class: 'etiqueta etiqueta-planejada' }, 'Planejada'));
    if (ideia.status === 'implemented' && ideia.implementedVersion) {
      meta.append(h('a', { class: 'etiqueta etiqueta-implementada', href: '#versoes' }, ideia.implementedVersion));
    }
    if (ideia.mine) meta.append(h('span', { class: 'etiqueta' }, 'Sua ideia'));
    if (ideia.reviewReason && (ideia.status === 'review')) {
      meta.append(h('span', { class: 'etiqueta etiqueta-alerta' }, MOTIVO[ideia.reviewReason] ?? ideia.reviewReason));
    }
    return h('li', { class: `ideia${compacto ? ' compacta' : ''}` },
      h('div', { class: 'placar', 'aria-label': `${ideia.score} pontos` },
        h('span', { class: 'placar-num' }, String(ideia.score)),
        h('span', { class: 'placar-rot' }, Math.abs(ideia.score) === 1 ? 'ponto' : 'pontos')),
      h('div', { class: 'ideia-corpo' },
        ideia.title ? h('h3', {}, ideia.title) : null,
        h('p', { class: 'ideia-texto' }, ideia.body),
        meta,
        compacto ? null : acoesAdmin(ideia)),
      botoesVoto(ideia));
  };

  const renderLista = () => {
    el.lista.replaceChildren(...estado.ideias.map((i) => itemIdeia(i)));
    if (!estado.ideias.length && !el.aviso.textContent) {
      el.aviso.textContent = {
        ranking: 'Nenhuma ideia publicada ainda. Que tal ser a primeira?',
        implemented: 'Nenhuma ideia implementada ainda.',
        review: 'Nada esperando moderação.',
      }[estado.aba];
    }
  };

  const renderAbas = () => {
    const abas = [['ranking', 'Mais votadas'], ['implemented', 'Implementadas']];
    if (estado.me?.isAdmin) abas.push(['review', 'Moderação']);
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
    ? h('p', { class: `ideia-msg ideia-msg-${estado.mensagem.tipo}`, role: estado.mensagem.tipo === 'erro' ? 'alert' : 'status' }, estado.mensagem.texto)
    : null);

  const renderComposer = () => {
    const c = el.composer;
    if (!estado.user) {
      trocar(c, 
        h('div', { class: 'composer-convite' },
          h('div', {},
            h('h3', {}, 'Tem uma ideia para o FFCom?'),
            h('p', {}, 'Entre com sua conta do FFCom para sugerir e votar. Uma sugestão por dia, com até 3 ajudas da varinha para deixar o texto claro.')),
          h('button', { type: 'button', class: 'btn', onclick: entrar }, 'Entrar para sugerir')),
        mensagem());
      return;
    }

    const topo = h('div', { class: 'composer-topo' },
      h('span', {}, `Olá, ${primeiroNome(estado.user)}`),
      h('button', { type: 'button', class: 'link-botao', onclick: sair }, 'Sair'));

    const me = estado.me;
    if (!me) {
      trocar(c, topo, h('p', { class: 'versoes-aviso' }, 'Carregando…'), mensagem());
      return;
    }

    if (me.suggestedToday && me.todayIdea) {
      const ideia = me.todayIdea;
      trocar(c, topo,
        h('div', { class: 'ideia-do-dia' },
          h('p', { class: 'ideia-do-dia-rot' }, `Sua sugestão de hoje · ${STATUS[ideia.status] ?? ideia.status}`),
          h('p', {}, ideia.body),
          h('p', { class: 'versoes-aviso' }, ideia.status === 'review'
            ? 'Ela aparece na lista assim que um moderador aprovar.'
            : 'Amanhã você pode enviar outra.')),
        mensagem());
      return;
    }

    const trabalhando = Boolean(estado.varinha);
    const usados = me.wandLimit - me.wandLeft;
    const campo = h('textarea', {
      id: 'ideia-texto',
      rows: '5',
      maxlength: String(MAX),
      placeholder: 'Ex.: poder fixar mensagens importantes no topo do canal.',
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
        ? `Melhorar o texto com o Claude. ${usados} de ${me.wandLimit} usos hoje`
        : 'Varinha indisponível no momento',
    }, svg(ICONES.varinha), h('span', {}, trabalhando ? 'Melhorando…' : 'Melhorar texto'),
    h('span', { class: 'varinha-contador', title: `${usados} de ${me.wandLimit} usos da varinha hoje` }, `${usados}/${me.wandLimit}`));
    const enviarBtn = h('button', { type: 'submit', class: 'btn' }, 'Enviar sugestão');

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
        h('p', { class: 'parecidas-rot' }, 'Parece com ideias que já existem. Se for a mesma coisa, vote nela; se for diferente, siga com a sua.'),
        h('ol', { class: 'ideias-lista' }, estado.parecidas.map((i) => itemIdeia(i, { compacto: true }))))
      : null;

    trocar(c, topo,
      h('form', { class: 'composer-form', onsubmit: enviar },
        h('label', { for: 'ideia-texto', class: 'composer-rot' }, 'Sua sugestão'),
        campo,
        h('div', { class: 'composer-barra' },
          varinha,
          estado.anterior != null && !trabalhando ? h('button', { type: 'button', class: 'link-botao', onclick: desfazer }, 'Desfazer') : null,
          contador,
          enviarBtn),
        trabalhando ? h('p', { class: 'versoes-aviso' }, 'O Claude está revisando seu texto. Quando a fila está cheia, pode levar alguns minutos.') : null,
        !me.assistEnabled ? h('p', { class: 'versoes-aviso' }, 'A varinha está indisponível agora; você ainda pode enviar sua sugestão.') : null),
      mensagem(),
      parecidas);
  };

  const render = () => {
    renderComposer();
    renderAbas();
  };

  // ---------- início

  const iniciar = async () => {
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
    el.aviso.textContent = 'Não foi possível carregar as ideias agora.';
  });
})();
