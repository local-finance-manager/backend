FROM golang:1.25-alpine AS builder

WORKDIR /app

# ca-certificates: a imagem final é scratch e não tem certificados raiz; sem eles
# toda chamada HTTPS ao googleapis.com falha com x509 (RNF-BKP-08).
RUN apk add --no-cache ca-certificates

# Download deps first (better layer cache)
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o server ./cmd/server

# -------------------------------------------------------
FROM scratch

# sem /etc/passwd no scratch; HOME garante que os.UserHomeDir() resolva corretamente
ENV HOME=/root

# Certificados raiz para o backup no Google Drive (HTTPS) — RNF-BKP-08.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 19742

ENTRYPOINT ["./server"]
