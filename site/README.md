# site

Home page pública do FFCom em `https://ffcom.a3sitsolutions.com.br/` (o `www.` redireciona para o apex). HTML e CSS puros, sem etapa de build: o que está neste diretório é o que o nginx serve.

## Rodando localmente

```
cp ../CHANGELOG.md .
docker build -t ffcom-site:local .
docker run --rm -p 8099:8080 ffcom-site:local
```

Ou abra `index.html` direto com qualquer servidor estático na raiz deste diretório (os caminhos são absolutos, `/styles.css`, `/assets/...`).

## Animações

A rede de servidores no fundo do hero é SVG animado (SMIL), e os pacotes do diagrama "Como funciona" são CSS. O `main.js` anima o chat do mock (alguém digitando, mensagens chegando) e o terminal (comandos digitados, saída aparecendo), e revela os cards ao rolar. É tudo enfeite: sem JavaScript o HTML já mostra o estado final do chat e do terminal.

Com `prefers-reduced-motion: reduce` (no Windows, "Efeitos de animação" desligado) somem a rede em movimento, os pacotes, os pulsos e os deslizamentos; o chat e o terminal continuam trocando de conteúdo, sem deslocamento. A saída do terminal imita o `docker compose up` do compose de referência e o `/healthz` real; se os nomes dos serviços ou a versão mudarem muito, vale atualizar o texto em `index.html`.

## Histórico de versões

A seção "Histórico de versões" lê `/changelog.md`, que o nginx busca do `CHANGELOG.md` do `main` no GitHub (cache de 5 minutos; se o GitHub falhar, serve o cache antigo ou a cópia embutida no build). O cabeçalho `X-Changelog-Source` diz de onde veio a resposta (`github (HIT)`, `github (MISS)`, `local`). O `CHANGELOG.md` não fica neste diretório no git: para buildar local, copie o da raiz antes (`cp ../CHANGELOG.md .`), como o workflow faz. O `nginx.conf` é um template da imagem oficial (vai para `/etc/nginx/templates/`), para o `resolver` usar o DNS do container.

## Identidade visual

- **Nome:** FFCom quer dizer *Friends & Family Communication*. O rodapé guarda o easter egg do "ff" dos jogos online (o voto para desistir da partida).
- **Marca:** `assets/logo.svg`, dois "f" minúsculos que dividem a mesma barra dentro de um balão de fala, em `#aa3bff` (o destaque do client). A mesma marca é o `client/public/favicon.svg`, de onde saem os ícones do PWA (`npm run generate:pwa-assets` em `client/`). Os `favicon.*` e `apple-touch-icon.png` daqui são cópias desses.
- **Fonte:** Bricolage Grotesque (títulos e wordmark), servida daqui mesmo em `assets/fonts/` (licença SIL OFL 1.1, `assets/fonts/OFL.txt`), sem Google Fonts. Só o subconjunto latin, que cobre o português.
- **Cores:** as mesmas do client (`client/src/index.css`), com tema claro e escuro por `prefers-color-scheme`.

`assets/og.png` (1200×630, prévia em redes sociais) foi renderizado a partir do SVG da marca; se a marca mudar, gere de novo.

## Deploy

Tag `site-vX.Y.Z` dispara `.github/workflows/deploy-ffcom-site.yml`: builda `ghcr.io/apolope/ffcom-site`, sobe `deploy/site/docker-compose.yml` (container `ffcom-site`, porta 8080, rede `a3s-services`) e espera o healthcheck. Ver `docs/architecture.md`, "Decisão: home page em `site/`".
