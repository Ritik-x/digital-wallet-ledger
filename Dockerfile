
# Build stage
FROM golang:1.27.0-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/ledgerx-api ./cmd/api

# Runtime stage
FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/ledgerx-api /app/ledgerx-api

USER app

EXPOSE 8080

ENTRYPOINT ["/app/ledgerx-api"]
