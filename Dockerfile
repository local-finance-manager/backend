FROM golang:1.22-alpine AS builder

WORKDIR /app

# Download deps first (better layer cache)
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o server ./cmd/server

# -------------------------------------------------------
FROM scratch

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 19742

ENTRYPOINT ["./server"]
