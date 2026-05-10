FROM golang:1-alpine AS builder
WORKDIR /build
COPY backend/ .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/server .

FROM alpine:latest AS signalserver
RUN apk add --no-cache \
    git \
    cmake \
    build-base \
    spdlog-dev \
    gdal-dev
WORKDIR /build
RUN git clone --depth 1 https://github.com/loorisr/Signal-Server.git .
RUN cmake -DCMAKE_CXX_FLAGS="-march=alderlake" -DCMAKE_C_FLAGS="-march=alderlake" src && make

FROM alpine:latest
RUN apk add --no-cache \
    gdal \
    spdlog \
    curl \
    ca-certificates
WORKDIR /app
COPY --from=builder /build/server /app/server
COPY --from=signalserver /build/signalserver /app/signalserver
COPY app/ui /app/app/ui
RUN chmod +x /app/server /app/signalserver
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD curl --fail http://localhost:8080/health || exit 1
CMD ["/app/server"]
