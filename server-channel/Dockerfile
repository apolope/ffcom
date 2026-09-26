# Imagem do container evergreen de server-channel (ver docs/architecture.md,
# "Decisão: container evergreen em `server-channel`"). A imagem não traz mais
# o serviço compilado aqui: traz o lançador ffcom-runtime (PID 1), que baixa,
# verifica e supervisiona o binário server-channel, e uma semente desse
# binário para o primeiro boot funcionar sem internet.
#
# Build args:
#   RUNTIME_VERSION          obrigatório; versão do contrato do container
#                            (X.Y.Z, >= server-channel/MIN_RUNTIME)
#   CHANNEL_VERSION          obrigatório; semente: X.Y.Z, X.Y ou latest
#                            (trilha resolvida no índice na hora do build)
#   FFCOM_RELEASE_INDEX_URL  opcional; índice alternativo (testes, forks)
#   FFCOM_RELEASE_PUBKEY     opcional; chave pública desse índice (base64)
#
# Multi-arquitetura sem QEMU: só os stages builder e seed executam binários,
# e os dois rodam na plataforma do builder ($BUILDPLATFORM). O Go compila o
# lançador cruzado para $TARGETOS/$TARGETARCH, install-seed baixa o artefato
# de $TARGETPLATFORM sem executá-lo, e o stage final não tem nenhum RUN.

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG RUNTIME_VERSION
WORKDIR /src
# O lançador só usa a stdlib e internal/release; o resto do módulo (e o .env
# de desenvolvimento, ver .dockerignore) fica fora da imagem de build.
COPY go.mod go.sum ./
COPY internal/release ./internal/release
COPY cmd/ffcom-runtime ./cmd/ffcom-runtime
# Dois lançadores com a mesma versão: /out/build roda aqui (install-seed no
# stage seguinte), /out/target vai para a imagem final.
RUN --mount=type=cache,target=/root/.cache/go-build \
    test -n "${RUNTIME_VERSION}" || { echo "defina --build-arg RUNTIME_VERSION=X.Y.Z" >&2; exit 1; }; \
    LDFLAGS="-s -w -X main.version=${RUNTIME_VERSION}"; \
    CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o /out/build/ffcom-runtime ./cmd/ffcom-runtime \
    && CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
       go build -trimpath -ldflags "${LDFLAGS}" -o /out/target/ffcom-runtime ./cmd/ffcom-runtime

# Semente: mesma verificação do auto-update (índice assinado, min_runtime do
# lançador, sha256). Com -platform diferente da do builder, install-seed não
# executa o --version do binário baixado. Os ARGs FFCOM_RELEASE_* chegam ao
# RUN como variáveis de ambiente; vazios, o lançador usa o índice e a chave
# embutidos.
#
# Com uma trilha (latest, X.Y) o cache do BuildKit não sabe que o índice
# mudou: o workflow resolve a trilha antes (--target seed-version) e builda a
# imagem com a versão exata, o que também põe a versão certa no LABEL.
FROM builder AS seed
ARG TARGETPLATFORM
ARG CHANNEL_VERSION
ARG FFCOM_RELEASE_INDEX_URL
ARG FFCOM_RELEASE_PUBKEY
RUN test -n "${CHANNEL_VERSION}" || { echo "defina --build-arg CHANNEL_VERSION=X.Y.Z|X.Y|latest" >&2; exit 1; }; \
    /out/build/ffcom-runtime install-seed -version "${CHANNEL_VERSION}" \
        -platform "${TARGETPLATFORM}" -dest /opt/ffcom/seed \
    && mkdir -p /rootfs/data/runtime /rootfs/data/attachments

# Só o VERSION da semente, para o workflow descobrir a versão resolvida:
#   docker buildx build --target seed-version --output type=local,dest=out .
FROM scratch AS seed-version
COPY --from=seed /opt/ffcom/seed/VERSION /VERSION

FROM alpine:3.20
ARG RUNTIME_VERSION
ARG CHANNEL_VERSION
LABEL org.opencontainers.image.title="ffcom-channel" \
      org.opencontainers.image.description="server-channel do FFCom: lançador ffcom-runtime com auto-update verificado" \
      org.opencontainers.image.source="https://github.com/apolope/ffcom" \
      org.opencontainers.image.version="${RUNTIME_VERSION}" \
      br.com.a3sitsolutions.ffcom.seed-version="${CHANNEL_VERSION}"
# Bundle de CAs do builder (Alpine mais novo) em vez de apk add: o stage final
# não pode ter RUN, senão buildar arm64 num host amd64 exigiria QEMU. O
# alpine:3.20 já traz ca-certificates-bundle; a cópia só o mantém atualizado.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/target/ffcom-runtime /usr/local/bin/ffcom-runtime
COPY --from=seed /opt/ffcom/seed /opt/ffcom/seed
# Usuário não-root por UID numérico (sem RUN adduser). /data/runtime e
# /data/attachments já nascem com esse dono: um volume nomeado vazio copia o
# dono do diretório da imagem na primeira montagem. Volume antigo criado como
# root, ou bind mount, precisa de chown 10001:10001 antes.
COPY --from=seed --chown=10001:10001 /rootfs/data /data
USER 10001:10001
VOLUME ["/data/runtime"]
EXPOSE 8080
# wget é o do busybox do Alpine. SERVER_PORT é a mesma variável do
# server-channel e do lançador; start-period cobre o primeiro boot (cópia da
# semente e migrations). sh -c em notação JSON porque a porta vem do
# ambiente do container.
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 \
    CMD ["/bin/sh", "-c", "wget -q -O /dev/null \"http://127.0.0.1:${SERVER_PORT:-8080}/healthz\" || exit 1"]
ENTRYPOINT ["/usr/local/bin/ffcom-runtime"]
