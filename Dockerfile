# Use the official Golang image as the base image
FROM golang:1.19 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the Go application
RUN go build -o main .

# Use a smaller base image for the final image
FROM gcr.io/distroless/base-debian10

# Copy the compiled binary from the builder stage
COPY --from=builder /app/main /

# Set the entry point for the container
ENTRYPOINT ["/main"]
