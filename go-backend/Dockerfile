FROM golang:1-alpine AS builder
WORKDIR /build
COPY go-backend/ .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/server .

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y \
    libgdal34 \
    libspdlog1.12 \
    curl \
    ca-certificates \
    && apt-get clean && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /build/server /app/server
COPY signalserver /app/signalserver
COPY app/ui /app/app/ui
RUN chmod +x /app/server /app/signalserver
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD curl --fail http://localhost:8080/health || exit 1
CMD ["/app/server"]
