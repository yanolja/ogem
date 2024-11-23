# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Explicitly disable CGO and build for better portability
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -ldflags="-w -s" \
    -o ogem cmd/main.go

FROM scratch

WORKDIR /app
COPY --from=builder /app/ogem .
COPY config.yaml .

ENV PORT=8080
EXPOSE ${PORT}

CMD ["/app/ogem"]