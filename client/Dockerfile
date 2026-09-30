FROM node:22-alpine AS builder
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Vite embute VITE_* no bundle em tempo de build — precisa ser passado como
# build arg, não como variável de ambiente do container em runtime.
ARG VITE_SERVER_CENTRAL_URL
ENV VITE_SERVER_CENTRAL_URL=$VITE_SERVER_CENTRAL_URL
# Versão da tag (ex. v0.17.3), mostrada no menu do avatar. Muda o bundle a
# cada release, o que também garante um sw.js novo para o botão de atualizar.
ARG VITE_APP_VERSION
ENV VITE_APP_VERSION=$VITE_APP_VERSION
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
