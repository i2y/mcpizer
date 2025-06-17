# Build stage
FROM golang:1.24-alpine AS builder

# Install buf for protobuf generation
RUN apk add --no-cache curl && \
    curl -sSL \
    "https://github.com/bufbuild/buf/releases/download/v1.32.1/buf-Linux-x86_64" \
    -o /usr/local/bin/buf && \
    chmod +x /usr/local/bin/buf

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Install protoc plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# Copy proto files and generate code
COPY buf.yaml buf.gen.yaml ./
COPY proto ./proto
RUN buf generate

# Copy source code
COPY . .

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -o calculator-server ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/calculator-server .

EXPOSE 50051

CMD ["./calculator-server"]