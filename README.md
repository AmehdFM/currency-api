# Currency API | Conversión e histórico de divisas

Servicio REST en Go que centraliza tasas de cambio de un proveedor externo, conserva su histórico y permite consultar conversiones desde una aplicación o integración.

## El problema que resuelve

Evita que cada cliente implemente por separado la descarga, almacenamiento y consulta de tasas. El servidor mantiene una copia en PostgreSQL y ofrece endpoints para tasas actuales, históricos y conversiones.

Las tasas dependen del proveedor configurado y de la última sincronización disponible; la API no garantiza cotizaciones en tiempo real.

## Funcionalidades

- Conversión entre divisas con aritmética decimal.
- Tasas actuales y consulta individual por moneda.
- Histórico con filtros de fechas y límite de resultados.
- Sincronización al arrancar y después cada 24 horas.
- Endpoint de estado que comprueba la conexión con la base.
- Creación de tablas al iniciar y contenedor Docker en varias etapas.

## Tecnologías y arquitectura

**Go · net/http · PostgreSQL · pgx/pool · shopspring/decimal · Docker**

| Componente | Responsabilidad |
|---|---|
| [main.go](main.go) | Composición y registro de rutas |
| [config/](config/) | Configuración de entorno |
| [database/](database/) | Pool de conexiones y tablas |
| [worker/](worker/) | Descarga periódica de tasas |
| [handlers/](handlers/) | Consultas y conversiones HTTP |
| [models/](models/) | Contrato del proveedor externo |
| [Dockerfile](Dockerfile) | Compilación y ejecución sobre scratch |

El uso de decimales permite representar y calcular montos sin recurrir a aritmética binaria de punto flotante en la conversión. Las respuestas se redondean según el endpoint; esto no sustituye las reglas de redondeo particulares de un sistema contable.

## Configuración y ejecución

Requisitos: Go compatible con [go.mod](go.mod), que declara Go 1.22, PostgreSQL y una fuente de tasas con el contrato descrito abajo. Crear previamente una base local `divisas_db`.

Comandos para Bash:

```bash
git clone https://github.com/AmehdFM/currency-api.git
cd currency-api
go mod download
export DATABASE_URL="postgres://usuario:password@localhost:5432/divisas_db?sslmode=disable"
export DATA_URL="https://proveedor.example.com/latest"
export PORT=8080
go run .
```

La URL del proveedor es ilustrativa: sustituirla por una fuente real compatible. `DATA_URL` es necesaria para poblar y actualizar las tasas, aunque el arranque solo valida explícitamente `DATABASE_URL`. No se carga automáticamente un archivo `.env`.

| Variable | Descripción |
|---|---|
| `DATABASE_URL` | Cadena de conexión a PostgreSQL |
| `DATA_URL` | URL de la fuente de tasas |
| `PORT` | Puerto HTTP, predeterminado `8080` |

### Contrato del proveedor

El worker lee pares con prefijo USD desde `quotes`:

```json
{
  "success": true,
  "timestamp": 1732000000,
  "quotes": {
    "USDUSD": 1,
    "USDEUR": 0.92,
    "USDHNL": 24.7
  }
}
```

Para convertir desde o hacia USD, la base también necesita la tasa de USD igual a 1; el worker no la agrega automáticamente si el proveedor no la devuelve. Cada sincronización registra el histórico con la hora de inserción de la base.

## API

| Método | Ruta | Función |
|---|---|---|
| GET | `/convert?from=USD&to=HNL&amount=100` | Conversión entre dos monedas almacenadas |
| GET | `/latest` | Todas las tasas actuales |
| GET | `/rates/HNL` | Tasa de una moneda |
| GET | `/history?code=HNL` | Histórico de una moneda, máximo 100 registros |
| GET | `/history?code=HNL&start=2026-01-01&end=2026-02-01` | Histórico filtrado |
| GET | `/check` | Estado de conexión PostgreSQL |

```bash
curl http://localhost:8080/check
curl http://localhost:8080/latest
curl "http://localhost:8080/convert?from=USD&to=HNL&amount=100"
```

La sincronización inicial se ejecuta en segundo plano; las consultas necesitan esperar a que existan tasas. `/check` verifica la base, no la frescura de los datos.

## Docker

Configurar las variables en la terminal, con un host PostgreSQL accesible desde el contenedor:

```bash
docker build -t currency-api .
docker run --rm -p 8080:8080 \
  -e DATABASE_URL -e DATA_URL -e PORT currency-api
```

## Estado y mejoras pendientes

Proyecto de backend para desarrollo y demostración, sin suite de pruebas automatizadas en el repositorio. Quedan por reforzar la validación de montos, fechas y tasas, el manejo de errores de escritura del worker, los tiempos de espera de las consultas externas y la supervisión de sincronizaciones.

Actualmente un monto inválido o igual a cero se trata como 1. Este comportamiento debe corregirse antes de usar la API en procesos financieros reales. Los endpoints no incorporan autenticación propia.

## Qué demuestra este proyecto

Integración con proveedores externos, procesos periódicos, persistencia histórica, consultas SQL parametrizadas, aritmética decimal y empaquetado de servicios Go.

## Autor

[Amehd Mendez](https://github.com/AmehdFM) · Backend e integración de sistemas.
