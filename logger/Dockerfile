FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

WORKDIR /app

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./

RUN go mod download

COPY main.go .

RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /server .

FROM scratch

COPY --from=builder /server /server

EXPOSE 5000

ENTRYPOINT ["/server"]
