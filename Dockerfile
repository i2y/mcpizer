# Dockerfile for MCPizer - primarily for development/testing
# For production use, install the binary directly with: go install github.com/i2y/mcpizer/cmd/mcpizer@latest

# Use the official Golang image to create a build artifact.
# This is known as a multi-stage build, it helps keep the final image size small.
FROM golang:1.24-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
# Assume the main package is in cmd/mcpizer and the output binary is named mcpizer
# TODO: Adjust if the main package location or output binary name is different.
RUN CGO_ENABLED=0 GOOS=linux go build -v -o mcpizer ./cmd/mcpizer


# Use a small base image for the final image
FROM alpine:latest

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/mcpizer .

# (Optional) Copy configs if needed by the application at runtime
# COPY --from=builder /app/configs ./configs

# Command to run the executable
# TODO: Add any necessary command-line flags if required
CMD ["./mcpizer"]
