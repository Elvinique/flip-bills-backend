# Flip Bills — Backend API

> Built with Go (Gin) · PostgreSQL · MongoDB · Redis

## Quick Start

```bash
# 1. Clone and enter the project
git clone <repo-url> && cd flip-bills

# 2. Install all Go dependencies
go mod tidy

# 3. Copy env config
cp .env.example .env   # then fill in secrets

# 4. Spin up infrastructure
make docker-up

# 5. Run the API
make run
```

The API will be live at `http://localhost:8080`

---

## Project Structure

```
flip-bills/
├── cmd/api/               # main.go — entrypoint & router wiring
├── internal/
│   ├── config/            # env-driven config loader
│   ├── handlers/          # HTTP handlers (auth, wallet, utilities, travel)
│   ├── middleware/         # JWT auth, rate limiting, KYC enforcement
│   ├── models/            # DB structs (User, Wallet, Transaction, TravelBooking)
│   ├── repository/
│   │   ├── postgres/      # ACID-safe SQL queries (pgx/v5)
│   │   └── mongo/         # Dynamic partner payload storage
│   ├── services/
│   │   ├── auth/          # Register, Login, Token refresh
│   │   ├── utilities/     # Airtime, Data, Electricity, Betting
│   │   ├── wallet/        # Balance, Fund, Transfer
│   │   ├── travel/        # Bus search/book, Flight GDS (Phase 2)
│   │   └── reconciliation/ # Async reversal + fallback aggregator engine
│   ├── queue/             # Background job dispatcher
│   └── notifications/     # SMS (Termii) + push notifications
├── pkg/
│   ├── jwt/               # Token generation & validation
│   ├── crypto/            # bcrypt hashing, OTP generation
│   ├── response/          # Unified API response envelope
│   └── logger/            # Structured zap logger
├── migrations/            # PostgreSQL SQL migration files
├── Dockerfile             # Multi-stage production image
├── docker-compose.yml     # Full local dev stack
└── Makefile               # Common dev tasks
```

---

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/register` | — | Create account + auto-provision wallet |
| POST | `/api/v1/auth/login` | — | Authenticate, receive JWT pair |
| POST | `/api/v1/vas/airtime` | Bearer | Buy airtime |
| POST | `/api/v1/vas/data` | Bearer | Buy data bundle |
| POST | `/api/v1/vas/electricity` | Bearer | Pay electricity (AEDC, EKEDC, etc.) |
| POST | `/api/v1/vas/betting` | Bearer | Fund betting wallet |
| GET | `/api/v1/wallet/balance` | Bearer | Get wallet balance |
| GET | `/api/v1/wallet/transactions` | Bearer | Transaction history |
| GET | `/api/v1/travel/bus/search` | Bearer | Search bus routes *(Phase 2)* |
| POST | `/api/v1/travel/bus/book` | Bearer | Book bus seat *(Phase 2)* |
| GET | `/api/v1/travel/flight/search` | Bearer | Search flights via GDS *(Phase 2)* |
| POST | `/api/v1/travel/flight/book` | Bearer | Book flight *(Phase 2)* |

---

## PRD Feature Coverage

| PRD Feature | Implementation File |
|-------------|-------------------|
| VAS Blackhole / Async Reconciliation | `internal/services/reconciliation/engine.go` |
| Offline Cryptographic QR Caching | `internal/models/travel.go` + Phase 2 |
| Betting Pre-flight Friction Prompt | `internal/services/utilities/utility_service.go` |
| KYC Tiered Daily Limits | `internal/repository/postgres/wallet_repo.go` |
| 3-Click Unified Checkout | `internal/services/utilities/` + `travel/` |
| Terminal Dispatcher Broadcast | Phase 3 — `internal/notifications/` |

---

## Development Phases (from PRD)

### Phase 1 — Core Infrastructure (Months 1–3) ✅
- [x] Wallet ledger (PostgreSQL, ACID)
- [x] KYC tiering + daily limits
- [x] JWT auth + bcrypt passwords
- [x] Airtime, Data, Electricity, Betting VAS
- [x] Async Reconciliation Engine
- [x] Redis rate limiting

### Phase 2 — Transit Integration (Months 4–7) 🔧
- [ ] Inter-state bus operator API integration
- [ ] GDS flight booking engine
- [ ] Offline QR cryptographic caching (SQLite in app)
- [ ] SMS fallback via Termii

### Phase 3 — B2B & Optimisation (Month 8+) 📋
- [ ] Terminal Dispatcher portal
- [ ] Loyalty rewards point system
- [ ] Velocity-based betting friction analytics
- [ ] Contextual cross-sell engine
