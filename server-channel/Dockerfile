FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server-channel .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/server-channel /usr/local/bin/server-channel
EXPOSE 8080
ENTRYPOINT ["server-channel"]
