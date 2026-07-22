# API de Divisas

API REST en Go para consulta y conversión de tipos de cambio, con histórico de tasas y sincronización automática diaria desde un proveedor externo.

## Características

- Conversión de montos entre divisas usando aritmética decimal exacta (sin errores de punto flotante)
- Consulta de todas las tasas actuales respecto a USD
- Consulta de una divisa específica
- Histórico de tasas por divisa, con filtro opcional por rango de fechas
- Sincronización automática cada 24 horas contra un proveedor externo de tipos de cambio
- Endpoint de healthcheck para monitoreo

## Stack técnico

| Componente     | Tecnología                                              |
|----------------|-----------------------------------------------------------|
| Lenguaje       | Go 1.22                                                    |
| Base de datos  | PostgreSQL (`pgx/v5` con pool de conexiones)               |
| Precisión numérica | `shopspring/decimal`                                   |

## Estructura del proyecto

```
mi-api-divisas/
├── main.go                # Punto de entrada: wiring de config, DB, worker y rutas HTTP
├── config/
│   └── config.go          # Carga de configuración desde variables de entorno
├── database/
│   └── database.go        # Conexión a PostgreSQL y creación de tablas
├── models/
│   └── models.go          # Struct de la respuesta del proveedor externo de tasas
├── worker/
│   └── worker.go          # Job en segundo plano que sincroniza tasas cada 24h
├── handlers/
│   └── handlers.go        # Endpoints HTTP (convert, history, latest, rates, check)
├── Dockerfile
└── go.mod
```

## Requisitos

- Go 1.22+
- PostgreSQL 13+
- Acceso a un proveedor externo de tipos de cambio que devuelva JSON en el formato descrito abajo

## Configuración

| Variable       | Requerida | Descripción                                                  | Valor por defecto |
|----------------|-----------|-----------------------------------------------------------------|--------------------|
| `DATABASE_URL` | Sí        | Cadena de conexión a PostgreSQL                                 | —                    |
| `DATA_URL`     | Sí        | URL del proveedor externo de tasas de cambio (base USD)         | —                    |
| `PORT`         | No        | Puerto donde escucha el servidor HTTP                            | `8080`               |

Ejemplo de `DATABASE_URL`:
```
postgres://usuario:password@localhost:5432/divisas_db?sslmode=disable
```

El proveedor configurado en `DATA_URL` debe responder con un JSON similar a:
```json
{
  "success": true,
  "timestamp": 1732000000,
  "quotes": {
    "USDEUR": 0.92,
    "USDMXN": 17.05,
    "USDHNL": 24.7
  }
}
```

## Instalación y ejecución

```bash
# Instalar dependencias
go mod tidy

# Definir variables de entorno
export DATABASE_URL="postgres://usuario:password@localhost:5432/divisas_db?sslmode=disable"
export DATA_URL="https://proveedor-de-tasas.example.com/latest"
export PORT=8080

# Ejecutar el servidor
go run main.go
```

Al iniciar, se crean automáticamente las tablas `exchange_rates` y `rate_history` si no existen, y se dispara una primera sincronización de tasas inmediatamente (luego se repite cada 24 horas en segundo plano).

## Docker

```bash
docker build -t api-divisas .
docker run \
  -e DATABASE_URL="postgres://usuario:password@host:5432/divisas_db" \
  -e DATA_URL="https://proveedor-de-tasas.example.com/latest" \
  -p 8080:8080 api-divisas
```

La imagen final se construye sobre `scratch` (sin sistema operativo base), por lo que el binario resultante es mínimo.

## API

Todos los endpoints devuelven `application/json`. No requieren autenticación.

### `GET /convert`
Convierte un monto de una divisa a otra.

**Query params:**
| Parámetro | Requerido | Descripción                                  |
|-----------|-----------|------------------------------------------------|
| `from`    | Sí        | Código de divisa origen (3 letras)             |
| `to`      | Sí        | Código de divisa destino (3 letras)            |
| `amount`  | No        | Monto a convertir (por defecto `1`)            |

**Ejemplo:** `GET /convert?from=USD&to=HNL&amount=100`
```json
{ "from": "USD", "to": "HNL", "amount": "100", "result": "2470.0000" }
```

### `GET /latest`
Devuelve todas las tasas actuales respecto a USD.
```json
{ "base": "USD", "rates": { "EUR": { "rate": "0.92", "updated_at": "..." } } }
```

### `GET /rates/{code}`
Devuelve la tasa actual de una divisa específica.

**Ejemplo:** `GET /rates/EUR`
```json
{ "code": "EUR", "rate": "0.92", "base": "USD", "updated_at": "..." }
```

### `GET /history`
Devuelve el histórico de tasas de una divisa (máximo 100 registros, más recientes primero).

**Query params:**
| Parámetro | Requerido | Descripción                                    |
|-----------|-----------|--------------------------------------------------|
| `code`    | Sí        | Código de divisa (3 letras)                       |
| `start`   | No        | Fecha/hora mínima (formato aceptado por Postgres) |
| `end`     | No        | Fecha/hora máxima (formato aceptado por Postgres) |

**Ejemplo:** `GET /history?code=EUR&start=2025-01-01&end=2025-01-31`
```json
[ { "code": "EUR", "rate": "0.921500", "date": "2025-01-15T00:00:00Z" } ]
```

### `GET /check`
Healthcheck del servicio y su conexión a base de datos.
```json
{ "status": "available", "database": "connected", "timestamp": "..." }
```
Devuelve `503` si la base de datos no responde.

## Modelo de datos

**exchange_rates** — última tasa conocida por divisa
| Campo          | Tipo                     |
|----------------|---------------------------|
| currency_code  | CHAR(3) PK                 |
| rate_to_base   | DECIMAL(18,8)               |
| updated_at     | TIMESTAMP WITH TIME ZONE    |

**rate_history** — histórico de tasas registradas
| Campo          | Tipo                     |
|----------------|---------------------------|
| id             | SERIAL PK                  |
| currency_code  | CHAR(3)                     |
| rate           | DECIMAL(18,8)                |
| recorded_at    | TIMESTAMP WITH TIME ZONE     |

## Notas

- Todos los cálculos monetarios usan `decimal.Decimal` en lugar de `float64` para evitar errores de redondeo.
- El worker de sincronización corre en una goroutine independiente y no bloquea el arranque del servidor HTTP.
- Los endpoints no tienen autenticación ni límite de peticiones (rate limiting); si la API va a exponerse públicamente, conviene añadir ambos antes de producción.
