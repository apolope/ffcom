# site

Home page pública do FFCom em `https://ffcom.a3sitsolutions.com.br/` (o `www.` redireciona para o apex). HTML e CSS puros, sem etapa de build: o que está neste diretório é o que o nginx serve.

## Rodando localmente

```
cp ../CHANGELOG.md ../CHANGELOG.en.md .
rm -rf locales && cp -r ../locales .
docker build -t ffcom-site:local .
docker run --rm -p 8099:8080 ffcom-site:local
```

Ou abra `index.html` direto com qualquer servidor estático na raiz deste diretório (os caminhos são absolutos, `/styles.css`, `/assets/...`).

## Animações

A rede de servidores no fundo do hero é SVG animado (SMIL), e os pacotes do diagrama "Como funciona" são CSS. O `main.js` anima o chat do mock (alguém digitando, mensagens chegando) e o terminal (comandos digitados, saída aparecendo), e revela os cards ao rolar. É tudo enfeite: sem JavaScript o HTML já mostra o estado final do chat e do terminal.

Com `prefers-reduced-motion: reduce` (no Windows, "Efeitos de animação" desligado) somem a rede em movimento, os pacotes, os pulsos e os deslizamentos; o chat e o terminal continuam trocando de conteúdo, sem deslocamento. A saída do terminal imita o `docker compose up` do compose de referência e o `/healthz` real; se os nomes dos serviços ou a versão mudarem muito, vale atualizar o texto em `index.html`.

## Idiomas

O site sai em português ou inglês. `i18n.js`, carregado sem `defer` no `<head>`, escolhe o idioma (o salvo no `localStorage` pela troca no rodapé, ou `navigator.languages`: `pt*` vira `pt-BR`, o resto `en`), busca `/locales/<idioma>.json` (os mesmos arquivos do client, copiados da raiz no build) e traduz a marcação: `data-i18n="chave"` (texto), `data-i18n-html="chave"` (texto com link ou negrito, só de `locales/`) e `data-i18n-attr="atributo:chave;..."`. Os scripts usam `window.ffcomI18n` (`t`, `errorText`, `lang`, `ready`, `onChange`); textos novos entram em `site.*` nos dois arquivos, e erros da API são mostrados por `errorText({code, message, params})`, que traduz `errors.<code>` e cai na `message`. O HTML continua em português: em português nada muda de lugar, e em outro idioma a página fica escondida (`html.i18n-pendente`, no máximo 3 segundos) até a tradução ser aplicada. `<title>`, descrição e Open Graph são trocados pelo script, então robôs que não executam JavaScript (prévias de link em redes sociais, por exemplo) sempre veem o português. `node scripts/check-locales.mjs` confere que toda chave usada aqui existe.

## Ideias

A seção "Ideias" (`ideias.js`) fala com o server-central (`data-central` na `<section id="ideias">`): lista pública de sugestões, e login OIDC no Authentik (provider `ffcom`, callback na própria raiz do site) para sugerir, usar a varinha e votar. O `oidc-client-ts` está copiado em `assets/vendor/` (licença Apache 2.0 ao lado) em vez de vir de CDN, pelo mesmo motivo da fonte; ao atualizar a versão do client, copie de `client/node_modules/oidc-client-ts/dist/browser/`. O login pede o scope `ffcom-groups`, que traz os grupos `ffcom-*` e diz ao server-central quem modera. As regras moram no server-central (ver `docs/architecture.md`, "Decisão: sugestões de melhoria com varinha do Claude").

## Histórico de versões

A seção "Histórico de versões" lê `/changelog.md` (ou `/changelog.en.md` com o site em inglês, relido na troca de idioma), que o nginx busca do `CHANGELOG.md` do `main` no GitHub (cache de 5 minutos; se o GitHub falhar, serve o cache antigo ou a cópia embutida no build). A tradução em inglês sai do mesmo jeito em `/changelog.en.md`, a partir do `CHANGELOG.en.md`, com cache e cópia de reserva próprios. O cabeçalho `X-Changelog-Source` diz de onde veio a resposta (`github (HIT)`, `github (MISS)`, `local`). Os dois CHANGELOGs e os arquivos de idioma (`locales/`) não ficam neste diretório no git: para buildar local, copie os da raiz antes (`cp ../CHANGELOG.md ../CHANGELOG.en.md .` e `cp -r ../locales .`), como o workflow faz. O `nginx.conf` é um template da imagem oficial (vai para `/etc/nginx/templates/`), para o `resolver` usar o DNS do container.

## Identidade visual

- **Nome:** FFCom quer dizer *Friends & Family Communication*. O rodapé guarda o easter egg do "ff" dos jogos online (o voto para desistir da partida).
- **Marca:** `assets/logo.svg`, dois "f" minúsculos que dividem a mesma barra dentro de um balão de fala, em `#aa3bff` (o destaque do client). A mesma marca é o `client/public/favicon.svg`, de onde saem os ícones do PWA (`npm run generate:pwa-assets` em `client/`). Os `favicon.*` e `apple-touch-icon.png` daqui são cópias desses.
- **Fonte:** Bricolage Grotesque (títulos e wordmark), servida daqui mesmo em `assets/fonts/` (licença SIL OFL 1.1, `assets/fonts/OFL.txt`), sem Google Fonts. Só o subconjunto latin, que cobre o português.
- **Cores:** as mesmas do client (`client/src/index.css`), com tema claro e escuro por `prefers-color-scheme`.

`assets/og.png` (1200×630, prévia em redes sociais) foi renderizado a partir do SVG da marca; se a marca mudar, gere de novo.

## Deploy

Tag `site-vX.Y.Z` dispara `.github/workflows/deploy-ffcom-site.yml`: builda `ghcr.io/apolope/ffcom-site`, sobe `deploy/site/docker-compose.yml` (container `ffcom-site`, porta 8080, rede `a3s-services`) e espera o healthcheck. Ver `docs/architecture.md`, "Decisão: home page em `site/`".
