// Seção "Participe": pedido de conta na instância oficial. Não exige login
// (quem pede ainda não tem conta). O pedido vai para o server-central, que
// o manda ao Telegram para aprovação; aprovado, o Authentik envia o e-mail
// de escolher a senha. As regras (formato dos campos, limites, pedidos
// repetidos) vivem no server-central; aqui só a interface. Ver
// docs/architecture.md, "Decisão: cadastro com aprovação pelo Telegram".
(() => {
  const secao = document.getElementById('participar');
  const form = document.getElementById('cadastro');
  const msg = document.getElementById('cadastro-msg');
  if (!secao || !form || !msg) return;

  const CENTRAL = secao.dataset.central;
  const USUARIO = /^[a-z0-9][a-z0-9._-]{2,29}$/;
  const botao = form.querySelector('button[type="submit"]');
  const campo = (nome) => form.elements.namedItem(nome);

  // O nome de usuário só aceita minúsculas sem acento; ajuda a pessoa
  // convertendo enquanto digita, em vez de recusar no envio.
  campo('username').addEventListener('input', (ev) => {
    const alvo = ev.target;
    const limpo = alvo.value.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/\s+/g, '.');
    if (limpo !== alvo.value) alvo.value = limpo;
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
    if (!USUARIO.test(d.username)) return ['username', 'O nome de usuário precisa ter de 3 a 30 caracteres: letras minúsculas sem acento, números, ponto, hífen ou sublinhado, começando por letra ou número.'];
    if (!/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(d.email)) return ['email', 'Confira o e-mail.'];
    if (tam(d.nickname) < 2 || tam(d.nickname) > 32) return ['nickname', 'O apelido precisa ter entre 2 e 32 caracteres.'];
    if (tam(d.reason) < 10 || tam(d.reason) > 500) return ['reason', 'Conte em 10 a 500 caracteres por que quer entrar.'];
    return null;
  };

  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const dados = {
      fullName: campo('fullName').value.trim(),
      username: campo('username').value.trim(),
      email: campo('email').value.trim(),
      nickname: campo('nickname').value.trim(),
      reason: campo('reason').value.trim(),
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
      const forte = document.createElement('strong');
      forte.textContent = 'Pedido enviado. ';
      const servidor = document.createElement('strong');
      servidor.textContent = 'sem um server-channel você só conseguirá trocar mensagens diretas';
      mostrar('ok', [
        forte,
        `Quando for aprovado, chega um e-mail em ${dados.email} para você escolher a senha. Enquanto isso, lembre: `,
        servidor,
        '. Para canais, voz e fórum, hospede o seu ou peça um convite a quem já tem um.',
      ]);
    } catch {
      mostrar('erro', 'Não foi possível falar com o servidor. Confira sua conexão e tente de novo.');
    } finally {
      botao.disabled = false;
      botao.textContent = 'Pedir minha conta';
    }
  });
})();
