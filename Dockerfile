FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build           -ldflags="-s -w"           -o webhook-filter           ./cmd/server

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/webhook-filter /webhook-filter

USER nonroot:nonroot

ENTRYPOINT ["/webhook-filter"]
CMD ["--config", "/config/config.yml"]
