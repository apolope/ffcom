FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /out/server-channel .

FROM alpine:3.20
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/server-channel /usr/local/bin/server-channel
EXPOSE 8080
ENTRYPOINT ["server-channel"]
