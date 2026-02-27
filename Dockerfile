# ---- Build stage ----
FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /mitz-replicator main.go

# ---- Certificate stage ----
FROM alpine:3.21 AS certs

RUN apk add --no-cache openssl
WORKDIR /certs

# Prevent path mangling in some environments
ENV MSYS_NO_PATHCONV=1

RUN set -e \
    && openssl req -x509 -newkey rsa:2048 -nodes \
         -keyout ca.key -out ca.crt -days 365 \
         -subj "/CN=Mitz Test CA" \
    && openssl req -newkey rsa:2048 -nodes \
         -keyout server.key -out server.csr \
         -subj "/CN=localhost" \
         -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
    && openssl x509 -req -in server.csr \
         -CA ca.crt -CAkey ca.key -CAcreateserial \
         -out server.crt -days 365 \
         -copy_extensions copyall \
    && openssl req -newkey rsa:2048 -nodes \
         -keyout client.key -out client.csr \
         -subj "/CN=mitz-connector" \
    && openssl x509 -req -in client.csr \
         -CA ca.crt -CAkey ca.key -CAcreateserial \
         -out client.crt -days 365 \
    && rm -f *.csr *.srl

# ---- Runtime stage ----
FROM alpine:3.21

RUN adduser -D -h /app appuser
WORKDIR /app

COPY --from=build /mitz-replicator .
COPY --from=certs /certs ./certs

RUN chown -R appuser:appuser /app
USER appuser

EXPOSE 8443

ENTRYPOINT ["./mitz-replicator"]
