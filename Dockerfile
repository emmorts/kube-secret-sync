# Start from the official Golang base image for building the binary
FROM golang:1.21 as builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o secret-clone-controller .

# Start a new stage from scratch for a smaller, final image
FROM alpine:latest  

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/secret-clone-controller .

# Command to run the executable
CMD ["./secret-clone-controller"]