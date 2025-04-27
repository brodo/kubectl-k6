# Build stage
FROM golang:latest AS build
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags  "-X github.com/brodo/kubectl-k6/cmd.version=$(git describe --tags)" -o kubectl-k6 .

# Final stage
FROM alpine:latest
RUN apk --no-cache add kubectl
COPY --from=build /app/kubectl-k6 /bin/kubectl-k6

