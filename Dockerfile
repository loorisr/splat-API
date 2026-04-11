# current signalserver binary has been compiled on ubuntu 24.04
FROM ubuntu:24.04

# Install system dependencies
RUN apt-get update && apt-get install -y \
    libgdal34 \
    libspdlog1.12 \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install uv and Python dependencies
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

COPY pyproject.toml .
# Disable development dependencies
ENV UV_NO_DEV=1
ENV UV_COMPILE_BYTECODE=1

RUN uv sync

# Copy application files
COPY . .

# Make precompiled binaries executable
RUN chmod +x /app/signalserver

EXPOSE 8080

CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8080"]
