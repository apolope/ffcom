# Design system

Padrões visuais do client (`client/`). Este documento começa pelas cores e
pelo scrollbar; tipografia, espaçamento e componentes entram aqui conforme
forem padronizados. A fonte da verdade é `client/src/index.css`: toda cor
listada aqui existe lá como variável CSS, e mudar um valor é mudar a
variável, nunca o CSS de um componente.

## Cores

### Regras

1. **Componente não escreve cor.** Todo `color`, `background`, `border` e
   `box-shadow` usa `var(--token)`. Se nenhum token serve, crie o token em
   `index.css` e documente aqui antes de usar.
2. **Token diz a função, não a aparência.** `--danger`, não `--vermelho`. É o
   que permite trocar a paleta ou o tema sem caçar usos.
3. **Tema segue o sistema.** O tema claro é o padrão de `:root` e o escuro
   sobrescreve em `@media (prefers-color-scheme: dark)`. Um token que muda
   entre os temas aparece nos dois blocos; um que não muda fica só em `:root`.
4. **Sem `var(--token, #fallback)`.** Os tokens estão sempre definidos, e o
   fallback só esconde erro de digitação no nome. (Até a criação deste
   documento, `--danger` nunca tinha sido definido e todos os usos caíam no
   fallback.)
5. **Contraste mínimo WCAG AA:** 4,5:1 para texto normal, 3:1 para texto
   grande (18px, ou 14px em negrito) e para ícones e bordas que carregam
   informação. Os pares que ainda não cumprem estão em "Pendências".

### Superfícies

Do fundo mais profundo para o mais elevado. A hierarquia é a do layout: o rail
fica no fundo, as colunas laterais um nível acima, a área de conteúdo no
`--bg`.

| Token | Claro | Escuro | Uso |
|---|---|---|---|
| `--bg-deep` | `#ebeaf0` | `#101014` | rail de servidores |
| `--bg` | `#ffffff` | `#16171d` | área principal (chat, voz, amigos), fundo da página, diálogos |
| `--bg-secondary` | `#f4f3ec` | `#1c1d24` | barra de canais, lista de membros, menus e notificações |
| `--bg-tertiary` | `#e5e4e7` | `#26272f` | ícones do rail, campos, itens em hover |
| `--code-bg` | `#f4f3ec` | `#1f2028` | `code` inline |
| `--border` | `#e5e4e7` | `#2e303a` | divisórias e bordas de campo |

### Texto

| Token | Claro | Escuro | Uso | Contraste sobre `--bg` |
|---|---|---|---|---|
| `--text-h` | `#08060d` | `#f3f4f6` | títulos, nomes, texto em destaque | 20,1 / 16,2 |
| `--text` | `#6b6375` | `#9ca3af` | texto corrido, rótulos, ícones | 5,7 / 7,0 |

### Acento

A cor da marca, roxo. Marca o que está ativo ou selecionado e a ação
principal de cada tela.

| Token | Claro | Escuro | Uso |
|---|---|---|---|
| `--accent` | `#aa3bff` | `#c084fc` | botão principal, servidor aberto, links, indicador de arrasto |
| `--accent-hover` | acento 70% + `#3b0a73` | idem | hover sobre um elemento que já está no acento |
| `--accent-bg` | acento a 10% | acento a 15% | fundo suave de item selecionado (definido, ainda sem uso) |
| `--accent-border` | acento a 50% | acento a 50% | contorno tracejado de alvo ao arrastar canal para uma categoria |
| `--on-accent` | `#ffffff` | `#101014` (`--bg-deep`) | texto e ícone sobre `--accent`, `--danger` ou `--success` |

### Semânticas

| Token | Claro | Escuro | Uso |
|---|---|---|---|
| `--danger` | `#da373c` | `#ed4245` | erro, ação destrutiva (excluir, banir), aviso de falha |
| `--success` | `#23a55a` | `#23a55a` | confirmação, atualização disponível, microfone aberto no push-to-talk |
| `--warning` | `#f0b232` | `#f0b232` | destaque de atenção (estrela do dono do servidor) |

### Status de presença

Pontos no canto do avatar (`AvatarWithStatus`). São as cores que usuários de
Discord já reconhecem, por isso não seguem a paleta semântica.

| Token | Valor | Status |
|---|---|---|
| `--status-online` | `#3ba55c` | online |
| `--status-busy` | `#ed4245` | ocupado |
| `--status-away` | `#faa61a` | ausente (escolhido ou por inatividade) |
| `--status-offline` | `#80848e` | offline e invisível |

### Sobreposições e mídia

| Token | Valor | Uso |
|---|---|---|
| `--overlay` | preto a 60% | fundo atrás de diálogo, máscara do recorte de avatar, rótulo sobre vídeo |
| `--media-bg` | `#000000` | fundo de vídeo (tarja do `object-fit: contain`) |
| `--on-media` | `#ffffff` | texto sobre vídeo ou sobre `--overlay` |
| `--shadow` | sombra dupla, mais forte no escuro | cartões |
| `--shadow-popover` | `0 8px 24px` preto a 35% | menus e notificações flutuantes |

A moldura branca a 85% do recorte de avatar (`AvatarCropper.css`) é a única
cor escrita direto num componente: fica sobre a foto do usuário e não depende
de tema.

### Cores que não são do sistema

A cor das roles (`roles.color` no server-channel) é escolhida por quem
administra o servidor e chega pela API. O client só a aplica ao nome na lista
de membros; ela não entra na paleta e não tem garantia de contraste.

## Scrollbar

Padrão fino, o mesmo de Discord, Slack e VS Code: sem setas, trilho
transparente, polegar arredondado no acento sobre o fundo da própria coluna.
Definido globalmente em `index.css`, então toda área com `overflow: auto` já
recebe o padrão sem código no componente.

| Propriedade | Valor |
|---|---|
| Largura total | 10px (a área de clique), fixa |
| Polegar visível | 2px parado, 4px com o mouse em cima, 6px rolando; cantos arredondados, altura mínima de 40px |
| Trilho | invisível parado, leve com o mouse em cima, cinza rolando |
| Cor do polegar | `--scrollbar-thumb`: `--accent` a 55% |
| Cor com o mouse sobre o polegar | `--scrollbar-thumb-hover`: `--accent` cheio |

A cor é o acento, a mesma das setas e divisórias pontilhadas do rail, para
tudo que indica "tem mais conteúdo" falar a mesma língua. Como deriva de
`--accent`, o scrollbar acompanha o tema sem precisar de valor próprio no
bloco escuro.

**Em teste: reagir a mouse e rolagem** (`client/src/lib/scrollbarActivity.ts`).
Uma espessura só (`--sb-size`) define os três estados, e a opacidade do
trilho cinza (`--sb-track`, sobre `--scrollbar-track`, `--text` a 20%)
acompanha:

| Estado | Polegar roxo | Trilho cinza |
|---|---|---|
| Parado | 2px, indica que a área rola | invisível |
| Mouse sobre a faixa do scrollbar | 4px | 40%, uma linha leve |
| Rolando | 6px | inteiro |

As transições levam 180ms para engrossar e 400ms para afinar; a rolagem conta
como terminada 700ms depois do último evento. O Chrome não aplica
`transition` nas partes do scrollbar, então a espessura é animada quadro a
quadro em JavaScript; a largura reservada fica fixa em 10px e o conteúdo não
se mexe. Para desfazer: remover a chamada de `installScrollbarActivity()` em
`main.tsx` e voltar as regras de `::-webkit-scrollbar` de `index.css` para a
versão fixa (polegar de 4px sempre visível, sem trilho).

**Navegadores.** Chromium e Electron usam `::-webkit-scrollbar`. O Firefox não
tem esses pseudo-elementos e cai em `scrollbar-width: thin` com
`scrollbar-color`, aplicado só dentro de `@supports not
selector(::-webkit-scrollbar)`. As duas formas não podem ser combinadas no
Chromium: a partir da versão 121, definir `scrollbar-color` ou
`scrollbar-width` desliga os pseudo-elementos, e no Windows as setas voltam.

**Sem barra: `.scroll-hidden`.** Para listas estreitas em que a barra rouba
espaço e a rolagem é óbvia pelo conteúdo. Hoje só o rail de servidores usa. A
roda do mouse, o toque e o teclado continuam funcionando. Não use em área de
conteúdo longo (chat, listas de membros): lá a barra é a única indicação de
posição.

## Pendências

Pares que não cumprem o contraste mínimo, medidos quando este documento foi
criado:

| Par | Contraste | Mínimo | Onde aparece |
|---|---|---|---|
| `--on-accent` sobre `--accent`, claro | 4,4 | 4,5 | botão principal no tema claro |
| `--on-accent` sobre `--success`, claro | 3,2 | 4,5 | botão de atualização em hover, push-to-talk ativo |
| `--on-accent` sobre `--accent-hover`, escuro | 4,4 | 4,5 | ícone do servidor aberto com o mouse em cima |
| `--warning` sobre `--bg`, claro | 1,9 | 3 | estrela do dono no tema claro |

Resolvido em 2026-09-25: no tema escuro, `--on-accent` passou de branco para
`--bg-deep`. Sobre o roxo o contraste foi de 2,6 para 7,2; sobre `--danger`,
de 3,8 para 4,9; sobre `--success`, de 3,2 para 6,0. O roxo da marca não
mudou.
