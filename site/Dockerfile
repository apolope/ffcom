# Home page estática em ffcom.a3sitsolutions.com.br: HTML/CSS/JS puros, sem
# etapa de build. CHANGELOG.md, CHANGELOG.en.md e locales/ não ficam neste
# diretório no git: o workflow (e quem builda local) copia os da raiz antes do
# build. Os dois CHANGELOGs viram as cópias de reserva do histórico de versões
# (ver nginx.conf, /changelog.md e /changelog.en.md); locales/ tem os arquivos
# de idioma, lidos por fetch (ver docs/architecture.md, "Decisão:
# internacionalização").
FROM nginx:1.27-alpine
# Preenche NGINX_LOCAL_RESOLVERS a partir do /etc/resolv.conf (ver nginx.conf).
ENV NGINX_ENTRYPOINT_LOCAL_RESOLVERS=1
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY index.html styles.css i18n.js main.js ideias.js cadastro.js favicon.svg favicon.ico apple-touch-icon.png /usr/share/nginx/html/
COPY CHANGELOG.md /usr/share/nginx/html/changelog-local.md
COPY CHANGELOG.en.md /usr/share/nginx/html/changelog-local.en.md
COPY assets /usr/share/nginx/html/assets
COPY locales /usr/share/nginx/html/locales
EXPOSE 8080
