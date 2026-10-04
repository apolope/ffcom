// Seção "Participe": pedido de conta na instância oficial. Não exige login
// (quem pede ainda não tem conta). O pedido vai para o server-central, que
// o manda ao Telegram para aprovação; aprovado, o Authentik envia o e-mail
// de escolher a senha. Quem já tem conta na infra A3S marca a caixa e não
// escolhe usuário nem apelido: a aprovação só dá acesso à conta que já
// existe. As regras (formato dos campos, nomes reservados, limites, pedidos
// repetidos) vivem no server-central; aqui só a interface. Ver
// docs/architecture.md, "Decisão: cadastro com aprovação pelo Telegram".
// Textos em site.signup.* e erros do server-central em errors.signup.* do
// arquivo de idioma (i18n.js).
(() => {
  const secao = document.getElementById('participar');
  const form = document.getElementById('cadastro');
  const msg = document.getElementById('cadastro-msg');
  const i18n = window.ffcomI18n;
  if (!secao || !form || !msg || !i18n) return;
  const { t } = i18n;

  const CENTRAL = secao.dataset.central;
  const USUARIO = /^[a-z0-9][a-z0-9._-]{2,29}$/;
  // Cópia de reservedUsernames do server-central (internal/httpapi/
  // signups.go), só para avisar antes; vale a de lá.
  const RESERVADOS = new Set([
    'a3s', 'a3sitsolutions', 'abuse', 'admin', 'administrador', 'administrator', 'ajuda', 'akadmin',
    'api', 'authentik', 'bot', 'contato', 'dono', 'equipe', 'ffcom', 'help', 'info', 'mod', 'moderacao',
    'moderador', 'moderator', 'noreply', 'null', 'oficial', 'official', 'owner', 'postmaster', 'root',
    'security', 'seguranca', 'sistema', 'staff', 'suporte', 'support', 'system', 'telegram', 'undefined',
    'webmaster', 'www',
  ]);
  // Os erros do server-central vêm em minúsculas e sem ponto final, para
  // caber no meio de uma frase; sozinhos na tela, ganham os dois.
  const frase = (texto) => (texto ? texto.charAt(0).toUpperCase() + texto.slice(1) + (/[.!?]$/.test(texto) ? '' : '.') : '');
  // Mesmos códigos e parâmetros que o server-central devolveria.
  const MSG_FORMATO = () => frase(i18n.errorText({ code: 'signup.username_format', params: { min: 3, max: 30 } }));
  const MSG_RESERVADO = () => frase(i18n.errorText({ code: 'signup.username_reserved' }));
  const botao = form.querySelector('button[type="submit"]');
  const campo = (nome) => form.elements.namedItem(nome);
  const statusUsuario = document.getElementById('cadastro-usuario-status');
  const dicaEmail = document.getElementById('cadastro-email-dica');
  const soContaNova = form.querySelectorAll('[data-conta-nova]');

  const jaTemConta = () => campo('existingAccount').checked;

  // Com a caixa marcada, usuário e apelido somem (a conta já tem os dela)
  // e a dica do e-mail passa a pedir o da conta existente.
  const aplicarModo = () => {
    const existente = jaTemConta();
    for (const el of soContaNova) {
      el.hidden = existente;
      for (const input of el.querySelectorAll('input')) input.disabled = existente;
    }
    dicaEmail.textContent = t(existente ? 'site.signup.emailHintExisting' : 'site.signup.emailHint');
    if (!botao.disabled) botao.textContent = t(existente ? 'site.signup.submitExisting' : 'site.signup.submit');
  };
  campo('existingAccount').addEventListener('change', aplicarModo);

  const problemaUsuario = (nome) => {
    if (!USUARIO.test(nome)) return MSG_FORMATO();
    if (RESERVADOS.has(nome.replace(/[._-]/g, ''))) return MSG_RESERVADO();
    return '';
  };

  const status = (tipo, texto) => {
    statusUsuario.className = `campo-status ${tipo}`;
    statusUsuario.textContent = texto;
  };

  // Confere o nome no server-central pouco depois de a pessoa parar de
  // digitar. É só aviso: o envio confere de novo, e falha aqui (rede,
  // limite de consultas) não impede nada.
  let espera = 0;
  let consulta = 0;
  const conferirDisponivel = (nome) => {
    clearTimeout(espera);
    const minha = ++consulta;
    if (!nome) return status('', '');
    const problema = problemaUsuario(nome);
    if (problema) return status('erro', problema);
    status('', t('site.signup.checking'));
    espera = setTimeout(async () => {
      try {
        const r = await fetch(`${CENTRAL}/api/signup-requests/username-available?username=${encodeURIComponent(nome)}`);
        if (minha !== consulta) return;
        if (!r.ok) return status('', '');
        // {available, code?, message, params?}: o motivo no formato dos
        // erros da API (docs/protocol.md, "Erros da API HTTP").
        const resposta = await r.json();
        if (minha !== consulta) return;
        if (resposta.available) status('ok', t('site.signup.available'));
        else status('erro', frase(i18n.errorText(resposta)));
      } catch {
        if (minha === consulta) status('', '');
      }
    }, 450);
  };

  // O nome de usuário só aceita minúsculas sem acento; ajuda a pessoa
  // convertendo enquanto digita, em vez de recusar no envio.
  campo('username').addEventListener('input', (ev) => {
    const alvo = ev.target;
    const limpo = alvo.value.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/\s+/g, '.');
    if (limpo !== alvo.value) alvo.value = limpo;
    conferirDisponivel(limpo);
  });

  const mostrar = (tipo, conteudo) => {
    msg.replaceChildren();
    if (!conteudo) return;
    const p = document.createElement('p');
    p.className = `ideia-msg ideia-msg-${tipo}`;
    p.setAttribute('role', tipo === 'erro' ? 'alert' : 'status');
    if (typeof conteudo === 'string') p.textContent = conteudo;
    else p.append(...conteudo);
    msg.append(p);
  };

  // Mesmas regras do server-central, para a pessoa não esperar a ida e
  // volta por um erro de digitação.
  const conferir = (d) => {
    const tam = (s) => [...s].length;
    if (tam(d.fullName) < 2 || tam(d.fullName) > 80) return ['fullName', t('site.signup.fullNameInvalid')];
    if (!d.existingAccount) {
      const problema = problemaUsuario(d.username);
      if (problema) return ['username', problema];
    }
    if (!/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(d.email)) return ['email', t('site.signup.emailInvalid')];
    if (!d.existingAccount && (tam(d.nickname) < 2 || tam(d.nickname) > 32)) return ['nickname', t('site.signup.nicknameLength', { min: 2, max: 32 })];
    if (tam(d.reason) < 10 || tam(d.reason) > 500) return ['reason', t('site.signup.reasonLength', { min: 10, max: 500 })];
    return null;
  };

  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const existingAccount = jaTemConta();
    const dados = {
      fullName: campo('fullName').value.trim(),
      username: existingAccount ? '' : campo('username').value.trim(),
      email: campo('email').value.trim(),
      nickname: existingAccount ? '' : campo('nickname').value.trim(),
      reason: campo('reason').value.trim(),
      existingAccount,
      website: campo('website').value,
      // Na aprovação vira o idioma da conta no Authentik (e-mail de
      // definir senha e telas de login).
      language: i18n.lang,
    };
    const problema = conferir(dados);
    if (problema) {
      mostrar('erro', problema[1]);
      campo(problema[0]).focus();
      return;
    }

    botao.disabled = true;
    botao.textContent = t('site.signup.sending');
    mostrar('info', t('site.signup.sendingRequest'));
    try {
      const r = await fetch(`${CENTRAL}/api/signup-requests`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(dados),
      });
      const texto = await r.text();
      if (!r.ok) {
        // Erro da API: {code, message, params?} (docs/protocol.md, "Erros da
        // API HTTP"), traduzido pelo code e, sem tradução, a message em
        // português; corpo que não é JSON (proxy na frente) vai cru.
        let erro = texto.trim();
        try {
          const problema = JSON.parse(texto);
          if (problema && typeof problema.message === 'string') erro = frase(i18n.errorText(problema));
        } catch {
          /* não é JSON */
        }
        mostrar('erro', erro || t('site.signup.failedStatus', { status: r.status }));
        return;
      }
      form.reset();
      status('', '');
      const forte = document.createElement('strong');
      forte.textContent = `${t('site.signup.sent')} `;
      const servidor = document.createElement('strong');
      servidor.textContent = t('site.signup.noServerChannel');
      // Quem já tem conta não recebe e-mail na aprovação (a conta já tem
      // senha), então o jeito de saber é tentar entrar.
      const quando = existingAccount
        ? t('site.signup.whenApprovedExisting')
        : t('site.signup.whenApproved', { email: dados.email });
      mostrar('ok', [
        forte,
        `${quando} `,
        servidor,
        `. ${t('site.signup.serverHint')}`,
      ]);
    } catch {
      mostrar('erro', t('site.signup.network'));
    } finally {
      botao.disabled = false;
      aplicarModo();
    }
  });

  // Os textos que o script põe na tela esperam o arquivo de idioma; uma
  // mensagem já mostrada fica no idioma em que apareceu.
  i18n.ready.then(aplicarModo);
  i18n.onChange(() => {
    aplicarModo();
    const nome = campo('username').value;
    if (nome && !jaTemConta()) conferirDisponivel(nome);
  });
})();
