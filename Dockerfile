FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with debug flag enabled
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X 'main.debug=true'" -o /bot ./cmd/bot

# Use a small alpine image for the final image
FROM alpine:3.19

WORKDIR /app

# Install necessary packages
RUN apk --no-cache add ca-certificates tzdata

# Copy the binary from the builder stage
COPY --from=builder /bot /app/bot

# Copy tools-spec directory
COPY tools-spec /app/tools-spec

# Create directories for temporary files
RUN mkdir -p /tmp/images

# Set environment variable to identify Docker Compose environment
ENV IN_DOCKER_COMPOSE=true

# Set the command to run the application
CMD ["/app/bot", "-debug"] 