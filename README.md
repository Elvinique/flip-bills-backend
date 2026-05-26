# Flip Bills Backend API

> "One app, one wallet, all your payments and travel sorted."

Flip Bills is a Go backend for a Nigerian lifestyle super-app that combines wallet-led payments, value-added services, travel booking, disruption handling, and loyalty rewards.

This repository currently contains the backend API, database migrations, partner adapter scaffolding, and local Docker infrastructure for PostgreSQL, MongoDB, and Redis.

## Current Status

The backend compiles and the current test suite passes.

```bash
GOCACHE=/tmp/flip-bills-gocache go test ./...
GOCACHE=/tmp/flip-bills-gocache go build ./cmd/api
```

Use the `GOCACHE` override if your local Go build cache is not writable from the current environment.

## Architecture

```text
Flutter Mobile App
        |
        | HTTPS / JSON
        v
Flip Bills Go API (Gin)
        |
        +-- Handlers: auth, wallet, VAS, travel, dispatcher, loyalty, webhooks
        +-- Services: domain orchestration and business rules
        +-- Repositories: Postgres ledger, Mongo travel cache
        +-- Packages: JWT, crypto, logger, API responses
        |
        +-- PostgreSQL: users, wallets, transactions, bookings, OTP, loyalty
        +-- MongoDB: dynamic travel search cache
        +-- Redis: rate limiting
        +-- Termii: SMS fallback
        +-- Flutterwave / Monnify: payments and webhooks
```

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.23 |
| HTTP framework | Gin |
| Primary database | PostgreSQL 16 via pgx/v5 |
| Dynamic cache | MongoDB 7 |
| Rate limiting | Redis 7 |
| Auth | JWT HS256 via golang-jwt/v5 |
| Password/PIN hashing | bcrypt |
| Logging | Uber Zap |
| SMS | Termii |
| Bills provider | Flutterwave client currently wired |
| Payment webhooks | Flutterwave and Monnify |
| Travel adapters | GIGM, ABC Transport, Amadeus scaffolds |
| Containers | Docker and Docker Compose |
| Migrations | golang-migrate compatible SQL files |

## Project Structure

```text
flip-bills-backend/
├── cmd/api/
│   └── main_with_loyalty.go        # API entrypoint, dependency wiring, routes
├── internal/
│   ├── config/                     # Environment config
│   ├── handlers/                   # HTTP handlers
│   ├── middleware/                 # JWT auth and Redis rate limiting
│   ├── models/                     # Domain models
│   ├── notifications/              # Termii SMS wrapper
│   ├── repository/
│   │   ├── mongo/                  # Travel search cache
│   │   └── postgres/               # User, wallet, travel, dispatcher, loyalty repos
│   └── services/
│       ├── auth/                   # Register, login, OTP, PIN, KYC
│       ├── wallet/                 # Balance, funding, transaction history
│       ├── utilities/              # VAS purchases and Flutterwave bills client
│       ├── reconciliation/         # VAS reversal/fallback engine
│       ├── travel/                 # Bus/flight search, booking, QR payloads
│       ├── dispatcher/             # Operator disruption flow
│       └── loyalty/                # Points earn, history, redeem
├── migrations/                     # Numbered up/down SQL migrations
├── pkg/
│   ├── crypto/                     # bcrypt, OTP, HMAC offline QR
│   ├── jwt/                        # JWT manager
│   ├── logger/                     # Zap factory
│   └── response/                   # Unified response helpers
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

## Getting Started

### Prerequisites

- Go 1.23+
- Docker and Docker Compose
- `golang-migrate` CLI, if you want to run migrations outside Docker

### 1. Configure environment

Create a `.env` file in the repository root. There is no committed `.env.example` yet, so use the variables below as the source of truth.

Required:

```env
JWT_SECRET=replace-with-32-byte-random-string
OFFLINE_QR_SECRET=replace-with-32-byte-random-string
POSTGRES_PASSWORD=supersecret
```

Common local defaults:

```env
APP_ENV=development
SERVER_PORT=8080
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=flipbills
POSTGRES_DB=flipbills_db
POSTGRES_SSL_MODE=disable
MONGO_URI=mongodb://localhost:27017
MONGO_DB=flipbills_dynamic
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
FLUTTERWAVE_SECRET_KEY=
FLUTTERWAVE_BASE_URL=https://api.flutterwave.com/v3
FLUTTERWAVE_WEBHOOK_SECRET=
MONNIFY_API_KEY=
MONNIFY_SECRET_KEY=
MONNIFY_BASE_URL=https://sandbox.monnify.com
TERMII_API_KEY=
TERMII_BASE_URL=https://api.ng.termii.com
RECONCILIATION_TIMEOUT_SECONDS=45
GIGM_API_KEY=
GIGM_BASE_URL=https://api.gigm.com/api
GIGM_DISPATCHER_KEY=
ABC_API_KEY=
ABC_BASE_URL=https://api.abctransport.com.ng
ABC_DISPATCHER_KEY=
AMADEUS_CLIENT_ID=
AMADEUS_CLIENT_SECRET=
AMADEUS_BASE_URL=https://test.api.amadeus.com
```

### 2. Start infrastructure

```bash
make docker-up
```

This starts PostgreSQL, MongoDB, Redis, and the API container.

For infrastructure only, use Docker Compose directly and omit/stop the API service as needed.

### 3. Run migrations

```bash
make migrate
```

The Makefile default database URL is:

```text
postgres://flipbills:supersecret@localhost:5432/flipbills_db?sslmode=disable
```

Important: the repository currently contains both numbered migrations and legacy full-schema files (`001_initial_schema.sql` and `001_initial_schema copy.sql`). Before a clean production-style migration run, keep only the numbered up/down migration sequence or move legacy schema dumps out of `migrations/`.

### 4. Run locally

```bash
make run
```

The API listens on `:8080` by default.

### 5. Health check

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"service":"flip-bills-api","status":"ok","version":"2.0.0"}
```

## API Overview

All application responses use a unified JSON envelope from `pkg/response`.

Base URL:

```text
http://localhost:8080
```

### Authentication

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | Public | Register user and create wallet |
| POST | `/api/v1/auth/login` | Public | Login and receive token pair |
| POST | `/api/v1/auth/verify-phone` | Public | Verify phone OTP |
| POST | `/api/v1/auth/resend-otp` | Public | Resend phone OTP |
| POST | `/api/v1/auth/refresh` | Public | Refresh access token |
| POST | `/api/v1/auth/set-pin` | Bearer | Set 6-digit transaction PIN |
| POST | `/api/v1/auth/kyc/upgrade` | Bearer | Upgrade with BVN or NIN |

Register:

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "+2348012345678",
    "password": "Password123!",
    "first_name": "Ada",
    "last_name": "Okonkwo"
  }'
```

Set transaction PIN:

```bash
curl -X POST http://localhost:8080/api/v1/auth/set-pin \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"pin":"123456","confirm_pin":"123456"}'
```

### Wallet

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/wallet/balance` | Bearer | Wallet balance and limits |
| GET | `/api/v1/wallet/transactions?page=1&limit=20` | Bearer | Paginated transaction history |
| POST | `/api/v1/wallet/fund` | Bearer | Manual/internal wallet credit after payment reference |

Wallet funding request:

```json
{
  "amount": 100000,
  "payment_ref": "gateway-reference",
  "provider": "flutterwave"
}
```

Amounts are stored in kobo. `100000` means NGN 1,000.

### Value-Added Services

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/vas/catalog` | Bearer | Flutterwave-backed or fallback VAS catalog |
| GET | `/api/v1/vas/transactions/:reference` | Bearer | Find a VAS transaction |
| POST | `/api/v1/vas/airtime` | Bearer | Buy airtime |
| POST | `/api/v1/vas/data` | Bearer | Buy data |
| POST | `/api/v1/vas/electricity` | Bearer | Pay electricity |
| POST | `/api/v1/vas/betting` | Bearer | Fund betting wallet |

Airtime:

```bash
curl -X POST http://localhost:8080/api/v1/vas/airtime \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "+2348012345678",
    "amount": 100000,
    "network": "MTN",
    "transaction_pin": "123456",
    "client_reference": "optional-idempotency-key"
  }'
```

Electricity:

```bash
curl -X POST http://localhost:8080/api/v1/vas/electricity \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "meter_number": "12345678901",
    "disco": "AEDC",
    "amount": 500000,
    "meter_type": "prepaid",
    "transaction_pin": "123456"
  }'
```

Betting top-ups include a pre-flight risk guard. If the amount is unusual for the user's recent behavior, the API returns a challenge requiring:

```json
{
  "risk_confirmed": true,
  "biometric_verified": true,
  "transaction_pin": "123456"
}
```

### Travel

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/travel/bus/search` | Bearer | Search bus routes |
| POST | `/api/v1/travel/bus/book` | Bearer | Book bus seat |
| GET | `/api/v1/travel/flight/search` | Bearer | Search flights |
| POST | `/api/v1/travel/flight/book` | Bearer | Book flight |
| GET | `/api/v1/travel/bookings` | Bearer | List my bookings |
| GET | `/api/v1/travel/bookings/:id` | Bearer | Get booking |
| POST | `/api/v1/travel/bookings/:id/reschedule` | Bearer | Reschedule disrupted booking |
| POST | `/api/v1/travel/bookings/:id/refund` | Bearer | Refund disrupted booking |

Bus search:

```bash
curl "http://localhost:8080/api/v1/travel/bus/search?origin=Lagos&destination=Abuja&departure_date=2026-06-01" \
  -H "Authorization: Bearer $TOKEN"
```

Bus booking:

```bash
curl -X POST http://localhost:8080/api/v1/travel/bus/book \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "operator_code": "GIGM",
    "vehicle_ref": "GIGM-BUS-0042",
    "seat_number": "3A",
    "departure_date": "2026-06-01",
    "origin": "Lagos",
    "destination": "Abuja",
    "passenger": {
      "full_name": "Ada Okonkwo",
      "phone": "+2348012345678",
      "email": "ada@example.com"
    }
  }'
```

Current limitation: bus and flight operator adapters still include stubbed mapping/booking behavior. They are good for local flow testing, not production inventory.

### Loyalty

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/loyalty/balance` | Bearer | Points balance and tier |
| GET | `/api/v1/loyalty/history` | Bearer | Points ledger |
| POST | `/api/v1/loyalty/redeem` | Bearer | Redeem points to wallet credit |

Redeem:

```bash
curl -X POST http://localhost:8080/api/v1/loyalty/redeem \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"points": 1000}'
```

### Webhooks

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/webhooks/flutterwave` | `verif-hash` header | Wallet funding callback |
| POST | `/webhooks/monnify` | `monnify-signature` header | Wallet funding callback |
| POST | `/webhooks/dispatcher` | API key in payload | Transport disruption event |

Dispatcher event:

```bash
curl -X POST http://localhost:8080/webhooks/dispatcher \
  -H "Content-Type: application/json" \
  -d '{
    "operator_code": "GIGM",
    "event_type": "vehicle_breakdown",
    "vehicle_ref": "GIGM-BUS-0042",
    "origin": "Lagos",
    "destination": "Abuja",
    "departure_time": "2026-06-01T07:00:00+01:00",
    "message": "Vehicle has broken down. Passengers may reschedule or request wallet credit.",
    "api_key": "raw-dispatcher-key"
  }'
```

## PRD Progress

### Achieved

| PRD Area | Current implementation |
|---|---|
| Unified wallet foundation | PostgreSQL wallet table, balance reads, wallet funding, daily spend limits, transaction ledger |
| User identity | Registration, login, JWT refresh, phone OTP records, transaction PIN, basic BVN/NIN tier upgrade |
| High-frequency VAS | Airtime, data, electricity, betting endpoints and service layer |
| VAS Blackhole handling | Reconciliation engine reverses failed debits; Flutterwave bill client is wired as the current provider |
| Betting over-funding protection | Rolling spend heuristic, explicit confirmation fields, biometric flag, and PIN requirement |
| Offline travel asset | HMAC-signed offline QR payload returned after booking |
| SMS fallback | Termii wrapper for OTP, booking confirmation, VAS success/refund, disruption alerts |
| Travel aggregation skeleton | Bus and flight search fan-out, normalized result models, Mongo search cache for bus routes |
| Terminal disruption flow | Dispatcher webhook, affected booking lookup, passenger disruption records, refund/reschedule endpoints |
| Loyalty | Loyalty accounts, points ledger, async VAS point awards, redemption to wallet credit |
| Deployment base | Dockerfile, docker-compose, Makefile, health check |

### Partially Achieved

| PRD Area | Gap |
|---|---|
| Backup aggregator routing | Engine supports fallback, but current VAS service passes no fallback provider yet |
| Real partner inventory | GIGM, ABC, and Amadeus adapters are scaffolded but still use placeholder response mapping or stubbed booking steps |
| Flight GDS | Amadeus auth/search request scaffold exists, but pricing and booking are stubbed |
| 3-click checkout cross-sell | Core routes exist, but contextual cross-sell recommendations are not implemented |
| Offline QR storage | Backend returns signed payload; Flutter app still needs local SQLite/Room storage |
| Production KYC | BVN/NIN fields update locally; no real identity provider integration yet |
| Payment funding flow | Webhooks credit wallet, but payment initialization/checkout endpoints are not yet implemented |

## Remaining Phases

### Phase 1: Core Infrastructure and VAS Stabilization

Status: mostly built, needs hardening.

- Clean migration strategy by removing legacy schema dumps from the active migration folder.
- Add `.env.example`.
- Add integration tests for auth, wallet, VAS, and webhook idempotency.
- Add payment initialization endpoints for Flutterwave/Monnify wallet funding.
- Add a real fallback bill provider path, or explicitly model Flutterwave as primary until a second provider is added.
- Improve transaction atomicity where wallet credit/debit and transaction insert should succeed or fail together.
- Confirm production-safe webhook signature behavior.

### Phase 2: Transit and Flight Integration

Status: scaffolded, not production ready.

- Replace GIGM and ABC placeholder mappings with real partner response decoding.
- Replace hardcoded bus booking price with live price validation from search/hold results.
- Implement real seat hold, confirm, and cancel calls for bus operators.
- Complete Amadeus flight offer decoding, pricing, and booking.
- Persist enough search/offer state to prevent client-side price tampering.
- Expand booking tests around refunds, failed confirmations, and offline QR verification.

### Phase 3: B2B Dispatcher Portal and Optimization

Status: backend event flow exists; portal and optimization remain.

- Build the Terminal Dispatcher web portal or operator-facing API layer.
- Add operator authentication and role management beyond shared dispatcher keys.
- Add push notification support alongside SMS.
- Add loyalty tier benefits and expiry jobs.
- Add contextual cross-sell engine for utilities, insurance, hospitality, and travel add-ons.
- Add observability: metrics, traces, dashboards, alerts, audit exports.

### Phase 4: Production Readiness

Status: pending.

- Secrets manager integration.
- CI pipeline for tests, linting, build, and migration checks.
- Database backups and restore drills.
- Load testing for wallet/VAS flows.
- Fraud/AML monitoring rules.
- Admin/support tooling for reconciliation, disputes, and manual reviews.

## Development

### Make targets

```bash
make run          # Start API locally
make build        # Compile binary to bin/server
make test         # Run tests with race detector
make tidy         # Sync go.mod and go.sum
make migrate      # Apply pending migrations
make migrate-down # Roll back one migration
make docker-up    # Start containers
make docker-down  # Stop containers
make lint         # Run golangci-lint
make env-check    # Check .env exists and warn about empty values
```

### Code conventions

- Store money as integer kobo.
- Use `DebitWithLock` for wallet debits.
- Keep user-facing HTTP formatting in handlers.
- Keep business rules in services.
- Keep SQL in repositories.
- Use `pkg/response` helpers for handler responses.
- Use Zap structured logging.
- Treat `status`, `external_ref`, and receipt `meta` as the only transaction fields currently updated after insert.

## Deployment Notes

The Dockerfile uses a multi-stage build and a final `scratch` image.

Production checklist:

- Set `APP_ENV=production`.
- Use managed PostgreSQL with TLS and `POSTGRES_SSL_MODE=require`.
- Move secrets out of `.env`.
- Configure real Flutterwave, Monnify, Termii, GIGM, ABC, and Amadeus credentials.
- Run migrations through a controlled release process.
- Schedule daily wallet spend reset.
- Add monitoring for `/health`.
- Enable database backups.

## License

MIT License. See `LICENSE`.
