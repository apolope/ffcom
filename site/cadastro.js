// Seção "Participe": pedido de conta na instância oficial. Não exige login
// (quem pede ainda não tem conta). O pedido vai para o server-central, que
// o manda ao Telegram para aprovação; aprovado, o Authentik envia o e-mail
// de escolher a senha. Quem já tem conta na infra A3S marca a caixa e não
// escolhe usuário nem apelido: a aprovação só dá acesso à conta que já
// existe. As regras (formato dos campos, nomes reservados, limites, pedidos
// repetidos) vivem no server-central; aqui só a interface. Ver
// docs/architecture.md, "Decisão: cadastro com aprovação pelo Telegram".
(() => {
  const secao = document.getElementById('participar');
  const form = document.getElementById('cadastro');
  const msg = document.getElementById('cadastro-msg');
  if (!secao || !form || !msg) return;

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
  const MSG_FORMATO = 'O nome de usuário precisa ter de 3 a 30 caracteres: letras minúsculas sem acento, números, ponto, hífen ou sublinhado, começando por letra ou número.';
  const MSG_RESERVADO = 'Esse nome de usuário é reservado; escolha outro.';
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
    dicaEmail.textContent = existente ? dicaEmail.dataset.existente : dicaEmail.dataset.nova;
    botao.textContent = existente ? 'Pedir acesso' : 'Pedir minha conta';
  };
  campo('existingAccount').addEventListener('change', aplicarModo);

  const problemaUsuario = (nome) => {
    if (!USUARIO.test(nome)) return MSG_FORMATO;
    if (RESERVADOS.has(nome.replace(/[._-]/g, ''))) return MSG_RESERVADO;
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
    status('', 'Conferindo...');
    espera = setTimeout(async () => {
      try {
        const r = await fetch(`${CENTRAL}/api/signup-requests/username-available?username=${encodeURIComponent(nome)}`);
        if (minha !== consulta) return;
        if (!r.ok) return status('', '');
        const { available, message } = await r.json();
        if (minha !== consulta) return;
        if (available) status('ok', 'Nome disponível.');
        else status('erro', message.charAt(0).toUpperCase() + message.slice(1) + '.');
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
    if (tam(d.fullName) < 2 || tam(d.fullName) > 80) return ['fullName', 'Informe o nome completo.'];
    if (!d.existingAccount) {
      const problema = problemaUsuario(d.username);
      if (problema) return ['username', problema];
    }
    if (!/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(d.email)) return ['email', 'Confira o e-mail.'];
    if (!d.existingAccount && (tam(d.nickname) < 2 || tam(d.nickname) > 32)) return ['nickname', 'O apelido precisa ter entre 2 e 32 caracteres.'];
    if (tam(d.reason) < 10 || tam(d.reason) > 500) return ['reason', 'Conte em 10 a 500 caracteres por que quer entrar.'];
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
    };
    const problema = conferir(dados);
    if (problema) {
      mostrar('erro', problema[1]);
      campo(problema[0]).focus();
      return;
    }

    botao.disabled = true;
    botao.textContent = 'Enviando...';
    mostrar('info', 'Enviando o pedido...');
    try {
      const r = await fetch(`${CENTRAL}/api/signup-requests`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(dados),
      });
      const texto = await r.text();
      if (!r.ok) {
        mostrar('erro', texto.trim() || `Não foi possível enviar o pedido (erro ${r.status}).`);
        return;
      }
      form.reset();
      status('', '');
      const forte = document.createElement('strong');
      forte.textContent = 'Pedido enviado. ';
      const servidor = document.createElement('strong');
      servidor.textContent = 'sem um server-channel você só conseguirá trocar mensagens diretas';
      // Quem já tem conta não recebe e-mail na aprovação (a conta já tem
      // senha), então o jeito de saber é tentar entrar.
      const quando = existingAccount
        ? 'Quando for aprovado, é só entrar no app com o usuário e a senha que você já usa na infra A3S (nenhum e-mail é enviado nesse caso). Enquanto isso, lembre: '
        : `Quando for aprovado, chega um e-mail em ${dados.email} para você escolher a senha. Enquanto isso, lembre: `;
      mostrar('ok', [
        forte,
        quando,
        servidor,
        '. Para canais, voz e fórum, hospede o seu ou peça um convite a quem já tem um.',
      ]);
    } catch {
      mostrar('erro', 'Não foi possível falar com o servidor. Confira sua conexão e tente de novo.');
    } finally {
      botao.disabled = false;
      aplicarModo();
    }
  });

  aplicarModo();
})();
