# ETAPA 1: Compilación
FROM golang:1.22-alpine AS builder

# Instalamos certificados y git
RUN apk add --no-cache ca-certificates git

WORKDIR /app

# 1. Copiamos el archivo de modulo
COPY go.mod ./

# 2. Copiamos el código fuente (necesario para que tidy sepa qué librerías usas)
COPY . .

# 3. Generamos go.sum y descargamos dependencias
RUN go mod tidy && go mod download

# 4. Compilamos el binario
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# ETAPA 2: Ejecución
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/main /main

EXPOSE 8080

ENTRYPOINT ["/main"]