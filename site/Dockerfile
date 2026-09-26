# Home page estática em ffcom.a3sitsolutions.com.br: HTML/CSS/JS puros, sem
# etapa de build. CHANGELOG.md não fica neste diretório no git: o workflow
# (e quem builda local) copia o da raiz antes do build, e ele vira a cópia
# de reserva do histórico de versões (ver nginx.conf, /changelog.md).
FROM nginx:1.27-alpine
# Preenche NGINX_LOCAL_RESOLVERS a partir do /etc/resolv.conf (ver nginx.conf).
ENV NGINX_ENTRYPOINT_LOCAL_RESOLVERS=1
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY index.html styles.css main.js favicon.svg favicon.ico apple-touch-icon.png /usr/share/nginx/html/
COPY CHANGELOG.md /usr/share/nginx/html/changelog-local.md
COPY assets /usr/share/nginx/html/assets
EXPOSE 8080
