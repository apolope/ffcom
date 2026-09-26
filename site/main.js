// Animações da home. Tudo aqui é enfeite: sem JavaScript a página mostra o
// mesmo conteúdo parado (o HTML já traz o estado final do chat e do
// terminal). Com prefers-reduced-motion some tudo que desloca coisas na
// tela (rede do hero, pulsos, cards subindo); o chat e o terminal continuam
// trocando de conteúdo, sem deslizar.
(() => {
  const reduzir = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  document.documentElement.classList.add('js');

  if (reduzir) {
    document.querySelectorAll('svg.rede').forEach((svg) => svg.pauseAnimations?.());
  }

  const espera = (ms) => new Promise((r) => setTimeout(r, ms));

  // Roda `fn` uma vez, quando `el` entra na tela.
  const aoAparecer = (el, fn, margem = '0px') => {
    if (!el) return;
    const io = new IntersectionObserver((entradas) => {
      if (entradas.some((e) => e.isIntersecting)) {
        io.disconnect();
        fn();
      }
    }, { rootMargin: margem });
    io.observe(el);
  };

  // Cards e diagrama sobem suavemente ao entrar na tela.
  if (!reduzir) document.querySelectorAll('.revelar').forEach((el, i) => {
    el.style.setProperty('--atraso', `${(i % 3) * 90}ms`);
    aoAparecer(el, () => el.classList.add('visivel'), '0px 0px -60px 0px');
  });

  // Chat do mock: alguém digita, a mensagem chega, a mais antiga sai.
  const chat = document.getElementById('mock-chat');
  const digitando = document.getElementById('mock-digitando');
  const conversa = [
    ['Ana', '#e8590c', 'Mandei as fotos da viagem no canal certo dessa vez.'],
    ['Marina', '#3ba55c', 'Ficaram lindas! A da praia vai pro porta-retrato.'],
    ['Tio Beto', '#faa61a', 'Achei a receita do bolo da vó. Posto em #receitas?'],
    ['Lucas', '#aa3bff', 'Posta! E o servidor tá rodando liso aqui em casa.'],
    ['Ana', '#e8590c', 'Melhor que grupo de mensagem: nada some e é tudo nosso.'],
    ['Marina', '#3ba55c', 'Vó entrou na chamada, corre lá!'],
  ];

  const novaMensagem = ([nome, cor, texto]) => {
    const msg = document.createElement('div');
    msg.className = 'mock-msg entrando';
    const av = document.createElement('span');
    av.className = 'av';
    av.style.setProperty('--c', cor);
    av.textContent = nome[0];
    const corpo = document.createElement('div');
    const b = document.createElement('b');
    b.textContent = nome;
    const p = document.createElement('p');
    p.textContent = texto;
    corpo.append(b, p);
    msg.append(av, corpo);
    return msg;
  };

  const rodarChat = async () => {
    let i = 0;
    for (;;) {
      if (document.hidden) { await espera(1000); continue; }
      const item = conversa[i % conversa.length];
      digitando.querySelector('b').textContent = item[0];
      digitando.classList.add('ativo');
      await espera(1600 + Math.random() * 900);
      digitando.classList.remove('ativo');

      const msgs = chat.querySelectorAll('.mock-msg');
      if (msgs.length >= 3) {
        const velha = msgs[0];
        velha.classList.add('saindo');
        await espera(280);
        velha.remove();
      }
      digitando.before(novaMensagem(item));
      i += 1;
      await espera(2200 + Math.random() * 1200);
    }
  };
  if (chat && digitando) aoAparecer(chat, rodarChat);

  // Terminal: digita os comandos e solta a saída, como numa instalação real.
  const code = document.getElementById('terminal-code');
  const rodarTerminal = async () => {
    const pre = code.parentElement;
    pre.style.minHeight = `${pre.offsetHeight}px`;
    const linhas = code.innerHTML.split('\n');
    code.innerHTML = '';
    const cursor = document.createElement('span');
    cursor.className = 'cursor';

    for (const linha of linhas) {
      const el = document.createElement('span');
      el.className = 'linha';
      code.append(el, cursor);
      const cmd = linha.match(/^(<span class="p">\$<\/span> )(.*)$/);
      if (cmd) {
        el.innerHTML = cmd[1];
        await espera(350);
        // Digita o texto do comando; um comentário no fim entra de uma vez.
        const [texto, comentario = ''] = cmd[2].split(/(?=\s*<span class="c">)/);
        const tmp = document.createElement('span');
        el.append(tmp);
        for (const ch of texto.replace(/&amp;/g, '&')) {
          tmp.textContent += ch;
          await espera(28 + Math.random() * 45);
        }
        el.insertAdjacentHTML('beforeend', comentario);
        await espera(420);
      } else {
        el.innerHTML = linha;
        await espera(linha.includes('Container') ? 380 : 160);
      }
      code.append(document.createTextNode('\n'));
    }
    code.append(cursor);
  };
  if (code) aoAparecer(code, rodarTerminal, '0px 0px -120px 0px');

  // Histórico de versões, lido do CHANGELOG.md (fonte única, no GitHub; o
  // nginx busca e guarda em cache, ver nginx.conf). Formato de cada seção:
  // "## <componente> v<X.Y.Z> · <AAAA-MM-DD>" seguida de itens "- ...".
  const COMPONENTES = {
    client: 'App',
    central: 'Central',
    channel: 'Servidor',
    'channel-image': 'Imagem do servidor',
    site: 'Site',
  };
  const lista = document.getElementById('versoes-lista');
  const atuais = document.getElementById('versoes-atuais');
  const filtros = document.getElementById('versoes-filtros');
  const mais = document.getElementById('versoes-mais');
  const INICIAIS = 6;

  const parseChangelog = (md) => {
    const cabecalho = /^## (client|central|channel|channel-image|site) v(\d+\.\d+\.\d+) · (\d{4}-\d{2}-\d{2})\s*$/;
    const versoes = [];
    let atual = null;
    let emCodigo = false;
    for (const linha of md.split(/\r?\n/)) {
      if (linha.startsWith('```')) { emCodigo = !emCodigo; continue; }
      if (emCodigo) continue;
      const h = linha.match(cabecalho);
      if (h) {
        atual = { comp: h[1], versao: h[2], data: h[3], itens: [] };
        versoes.push(atual);
      } else if (linha.startsWith('## ')) {
        atual = null;
      } else if (atual && linha.startsWith('- ')) {
        atual.itens.push(linha.slice(2).trim());
      }
    }
    return versoes.filter((v) => v.itens.length);
  };

  // Só `código` e **negrito**; todo o resto vira texto.
  const inline = (texto) => {
    const frag = document.createDocumentFragment();
    for (const parte of texto.split(/(`[^`]+`|\*\*[^*]+\*\*)/)) {
      if (!parte) continue;
      if (parte.startsWith('`') && parte.endsWith('`') && parte.length > 1) {
        const c = document.createElement('code');
        c.textContent = parte.slice(1, -1);
        frag.append(c);
      } else if (parte.startsWith('**') && parte.endsWith('**') && parte.length > 3) {
        const b = document.createElement('strong');
        b.textContent = parte.slice(2, -2);
        frag.append(b);
      } else {
        frag.append(parte);
      }
    }
    return frag;
  };

  const fmtData = new Intl.DateTimeFormat('pt-BR', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' });
  const selo = (comp) => {
    const s = document.createElement('span');
    s.className = `comp comp-${comp}`;
    s.textContent = COMPONENTES[comp];
    return s;
  };

  const renderVersoes = (versoes) => {
    let filtro = 'todos';
    let expandido = false;

    // Versão atual de cada componente: a primeira que aparece no arquivo.
    atuais.replaceChildren();
    for (const comp of Object.keys(COMPONENTES)) {
      const v = versoes.find((x) => x.comp === comp);
      if (!v) continue;
      const chip = document.createElement('span');
      chip.className = 'versao-atual';
      const num = document.createElement('b');
      num.textContent = `v${v.versao}`;
      chip.append(selo(comp), num);
      atuais.append(chip);
    }

    const desenhar = () => {
      const visiveis = versoes.filter((v) => filtro === 'todos' || v.comp === filtro);
      const mostrar = expandido ? visiveis : visiveis.slice(0, INICIAIS);
      lista.replaceChildren(...mostrar.map((v) => {
        const li = document.createElement('li');
        li.className = 'versao';
        const topo = document.createElement('div');
        topo.className = 'versao-topo';
        const num = document.createElement('span');
        num.className = 'versao-num';
        num.textContent = `v${v.versao}`;
        const data = document.createElement('time');
        data.dateTime = v.data;
        data.textContent = fmtData.format(new Date(`${v.data}T00:00:00Z`));
        topo.append(selo(v.comp), num, data);
        const ul = document.createElement('ul');
        for (const item of v.itens) {
          const it = document.createElement('li');
          it.append(inline(item));
          ul.append(it);
        }
        li.append(topo, ul);
        return li;
      }));
      const resto = visiveis.length - mostrar.length;
      mais.hidden = resto <= 0 && !expandido;
      mais.textContent = expandido ? 'Mostrar menos' : `Mostrar mais ${resto} ${resto === 1 ? 'versão' : 'versões'}`;
    };

    const opcoes = [['todos', 'Todas'], ...Object.entries(COMPONENTES).filter(([c]) => versoes.some((v) => v.comp === c))];
    filtros.replaceChildren(...opcoes.map(([valor, rotulo]) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'filtro';
      b.textContent = rotulo;
      b.setAttribute('aria-pressed', String(valor === filtro));
      b.addEventListener('click', () => {
        filtro = valor;
        expandido = false;
        filtros.querySelectorAll('button').forEach((x) => x.setAttribute('aria-pressed', String(x === b)));
        desenhar();
      });
      return b;
    }));
    filtros.hidden = false;

    mais.addEventListener('click', () => {
      const estavaExpandido = expandido;
      expandido = !expandido;
      desenhar();
      if (estavaExpandido) document.getElementById('versoes').scrollIntoView();
    });

    desenhar();
  };

  const carregarVersoes = async () => {
    const aviso = lista.innerHTML;
    lista.innerHTML = '<li class="versoes-aviso">Carregando o histórico…</li>';
    try {
      const r = await fetch('/changelog.md', { headers: { Accept: 'text/plain' } });
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      const versoes = parseChangelog(await r.text());
      if (!versoes.length) throw new Error('CHANGELOG sem versões');
      renderVersoes(versoes);
    } catch (e) {
      console.warn('Histórico de versões indisponível:', e);
      lista.innerHTML = aviso;
    }
  };
  if (lista) aoAparecer(lista, carregarVersoes, '0px 0px 400px 0px');
})();
