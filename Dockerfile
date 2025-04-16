FROM golang:1.23.3-alpine3.20 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Explicitly disables CGO and builds for better portability.
RUN CGO_ENABLED=0 go build -o ogem cmd/main.go

# Same version used in the Golang image.
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/ogem .
COPY config.yaml .

ENV PORT=8080
EXPOSE ${PORT}

CMD ["/app/ogem"]
