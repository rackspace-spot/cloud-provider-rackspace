# Build stage
FROM golang:1.22.4 AS builder

WORKDIR /app

# Copy the entire project directory
COPY . .

# If you have a local replacement for gophercloud, uncomment and modify the following line:
# COPY ../gophercloud /gophercloud

# Download all dependencies
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build \
    -o rackspace-cloud-controller-manager \
    cmd/openstack-cloud-controller-manager/main.go

# Runtime stage
FROM alpine:3.18.2

RUN apk add --no-cache gcompat ca-certificates

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/rackspace-cloud-controller-manager /bin/cloud-controller-manager

ENTRYPOINT ["/bin/cloud-controller-manager"]
