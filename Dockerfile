FROM golang:1.21-alpine AS builder

# Install build dependencies for SQLite
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

# Copy go files
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build binary with CGO enabled (required for sqlite3)
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o bot .

# Final stage
FROM alpine:3.19

RUN apk add --no-cache sqlite-libs ca-certificates tzdata

WORKDIR /app

# Copy binary and config
COPY --from=builder /app/bot .
COPY config.json .

# Create store directory
RUN mkdir -p store

EXPOSE 8080

CMD ["./bot"]
