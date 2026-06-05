FROM golang:1.22-alpine AS builder

# Install build dependencies for SQLite
RUN apk add --no-cache gcc musl-dev sqlite-dev git

WORKDIR /app

# Copy module files and download dependencies
# GONOSUMDB=* and GOFLAGS=-mod=mod allow go.sum to be populated at build time
COPY go.mod go.sum* ./
RUN GONOSUMDB=* GOFLAGS=-mod=mod go mod download

COPY . .

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux GONOSUMDB=* GOFLAGS=-mod=mod go build -ldflags="-w -s" -o bot .

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
