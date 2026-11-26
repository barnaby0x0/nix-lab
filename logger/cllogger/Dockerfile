FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

WORKDIR /app

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./

RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /server ./cmd/logger

FROM scratch

# Certificats CA : nécessaires si l'endpoint OTLP est en HTTPS (absents de scratch)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /server /server

EXPOSE 5000

ENTRYPOINT ["/server"]
