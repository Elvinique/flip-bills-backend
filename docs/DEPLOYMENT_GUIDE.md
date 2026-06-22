# Flip Bills — Deployment Guide

> **Version**: 1.0 | **Last Updated**: June 2026 | **Environment**: Production

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Environment Variables](#2-environment-variables)
3. [Local Development](#3-local-development)
4. [Database Setup & Migrations](#4-database-setup--migrations)
5. [Docker Deployment (Single Host)](#5-docker-deployment-single-host)
6. [Kubernetes Deployment](#6-kubernetes-deployment)
7. [Monitoring Stack](#7-monitoring-stack)
8. [CI/CD Pipeline](#8-cicd-pipeline)
9. [Secrets Management](#9-secrets-management)
10. [Rollback Procedures](#10-rollback-procedures)
11. [Health Checks & Smoke Tests](#11-health-checks--smoke-tests)
12. [Troubleshooting](#12-troubleshooting)

---

## 1. Prerequisites

| Tool | Minimum Version | Notes |
|------|----------------|-------|
| Go | 1.23+ | Backend language |
| Docker | 24+ | Container runtime |
| Docker Compose | 2.27+ | Multi-service orchestration |
| kubectl | 1.29+ | Kubernetes CLI (production only) |
| golang-migrate | 4.17+ | DB schema migrations |
| Postgres | 16 | Primary database |
| Redis | 7 | Cache / rate limiting |
| RabbitMQ | 3.13 | Async job queue |

Install `golang-migrate`:
```bash
curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.1/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/
```

---

## 2. Environment Variables

Copy the example file and fill in real values:

```bash
cp .env.example .env
```

### Required Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL DSN | `postgres://user:pass@host:5432/db?sslmode=require` |
| `REDIS_URL` | Redis DSN | `redis://:password@host:6379` |
| `AMQP_URL` | RabbitMQ DSN | `amqp://user:pass@host:5672/` |
| `JWT_SECRET` | HS256 signing key (≥32 chars) | `openssl rand -hex 32` |
| `PAYSTACK_SECRET_KEY` | Paystack live key | `sk_live_...` |
| `OPAY_SECRET_KEY` | OPay HMAC key | |
| `OPAY_PUBLIC_KEY` | OPay public key | |
| `OPAY_MERCHANT_ID` | OPay merchant ID | |
| `PORT` | API listen port | `8080` |
| `ENVIRONMENT` | `development` / `staging` / `production` | `production` |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GRAFANA_USER` | `admin` | Grafana admin username |
| `GRAFANA_PASSWORD` | `flipbills_grafana_secret` | Grafana admin password |
| `RABBITMQ_USER` | `flipbills` | RabbitMQ username |
| `RABBITMQ_PASS` | `flipbills_rabbit_secret` | RabbitMQ password |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

> [!CAUTION]
> Never commit `.env` to version control. Rotate all secrets immediately if accidentally exposed.

---

## 3. Local Development

```bash
# 1. Clone the repo
git clone https://github.com/<org>/flip-bills-backend.git
cd flip-bills-backend

# 2. Copy env file
cp .env.example .env
# Edit .env with local values

# 3. Start infrastructure (Postgres, Redis, Mongo)
docker compose up -d postgres redis mongo

# 4. Run migrations
make migrate-up
# or manually:
migrate -path ./migrations -database "$DATABASE_URL" up

# 5. Start the API
go run ./cmd/api
# or
make run
```

The API is now available at `http://localhost:8080`.

---

## 4. Database Setup & Migrations

### Running Migrations

```bash
# Apply all pending migrations
make migrate-up

# Rollback last migration
make migrate-down

# Check current version
migrate -path ./migrations -database "$DATABASE_URL" version

# Force a specific version (use only to fix dirty state)
migrate -path ./migrations -database "$DATABASE_URL" force <version>
```

### Migration Files

Located in `./migrations/`. Numbered sequentially `000001` → `000015`.

| Migration | Purpose |
|-----------|---------|
| `000001` | Users |
| `000002` | Wallets |
| `000003` | Transactions |
| `000004` | Travel bookings |
| `000005` | Daily spend reset cron |
| `000006` | `updated_at` triggers |
| `000007` | OTP tokens |
| `000008` | Dispatcher events |
| `000009` | Loyalty points |
| `000010` | External ref unique index |
| `000011` | Commission column |
| `000012` | Ledger entries |
| `000013` | Transfers |
| `000014` | Virtual accounts |
| `000015` | Webhook events |

> [!IMPORTANT]
> Always run migrations **before** deploying a new API version. The CI pipeline does this automatically via `kubectl exec` for Kubernetes deployments.

---

## 5. Docker Deployment (Single Host)

Best suited for staging or low-traffic environments.

```bash
# Start main services (Postgres, Redis, Mongo, API)
docker compose up -d

# Add monitoring stack
docker compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d

# View logs
docker compose logs -f api

# Stop everything
docker compose down
```

### Service Ports (default)

| Service | Port |
|---------|------|
| API | `8080` |
| PostgreSQL | `5432` |
| Redis | `6379` |
| MongoDB | `27017` |
| RabbitMQ AMQP | `5672` |
| RabbitMQ Management | `15672` |
| Prometheus | `9090` |
| Grafana | `3000` |

### Building the API Image Locally

```bash
docker build -t flip-bills-api:local .

# Run standalone (for testing)
docker run --rm --env-file .env -p 8080:8080 flip-bills-api:local
```

---

## 6. Kubernetes Deployment

### Prerequisites

- A running Kubernetes cluster (GKE / EKS / AKS / bare-metal)
- `kubectl` configured with appropriate context
- Secrets pre-created in the `flip-bills` namespace

### Namespace & Secrets Setup

```bash
# Create namespace
kubectl create namespace flip-bills

# Create secret from .env
kubectl create secret generic flip-bills-secrets \
  --from-env-file=.env \
  -n flip-bills
```

### Deploy the API

```bash
# Apply the Kubernetes manifests
kubectl apply -f k8s/ -n flip-bills

# Verify rollout
kubectl rollout status deployment/flipbills-api -n flip-bills

# Check pods
kubectl get pods -n flip-bills
```

### The `k8s/api.yaml` manifest provisions:

- **Deployment** — 2 replicas, rolling update strategy, resource limits
- **Service** — ClusterIP on port 8080
- **HorizontalPodAutoscaler** — scales 2→10 replicas on CPU >60%

### Scaling Manually

```bash
kubectl scale deployment flipbills-api --replicas=5 -n flip-bills
```

### Rolling Update (manual)

```bash
IMAGE="ghcr.io/<org>/flip-bills-backend:v1.2.0"
kubectl set image deployment/flipbills-api api=$IMAGE -n flip-bills
kubectl rollout status deployment/flipbills-api -n flip-bills --timeout=300s
```

---

## 7. Monitoring Stack

### Quick Start (Docker)

```bash
docker compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d
```

| Component | URL | Default Credentials |
|-----------|-----|---------------------|
| Grafana | http://localhost:3000 | admin / `$GRAFANA_PASSWORD` |
| Prometheus | http://localhost:9090 | — |
| RabbitMQ UI | http://localhost:15672 | flipbills / `$RABBITMQ_PASS` |

### Dashboards

The **Flip Bills Production Dashboard** (`flip_bills.json`) is auto-provisioned and includes:

- **API Overview** — uptime status, request rate by status code, P50/P95/P99 latency
- **Business Metrics** — transfer success/failure rate, VAS purchases by service type
- **Infrastructure** — PostgreSQL connections, Redis memory, RabbitMQ queue depth

### Alerts

Configured in `monitoring/rules/alerts.yml`. Key alerts:

| Alert | Condition | Severity |
|-------|-----------|----------|
| `APIDown` | API unreachable >1m | Critical |
| `HighP99Latency` | P99 > 2s for 5m | Warning |
| `HighErrorRate` | 5xx > 5% for 5m | Critical |
| `TransferFailureSpike` | >0.5 failures/s | Warning |
| `PostgresDown` | Exporter unreachable | Critical |
| `RedisMemoryHigh` | >90% memory | Warning |
| `DiskSpaceLow` | <15% disk on `/` | Warning |

To receive alert notifications, configure `alerting.alertmanagers` in `monitoring/prometheus.yml` with your Alertmanager host.

---

## 8. CI/CD Pipeline

The pipeline (`.github/workflows/ci.yml`) has **6 jobs**:

```
lint ──┐
       ├──► docker ──► deploy-staging ──► deploy-production
test ──┤                                  (on release only)
       │
security
```

| Job | Trigger | Actions |
|-----|---------|---------|
| `lint` | All PRs & pushes | golangci-lint |
| `test` | All PRs & pushes | Unit + integration tests with real Postgres/Redis, coverage upload to Codecov |
| `security` | All PRs & pushes | govulncheck, gosec, Trivy FS scan |
| `docker` | push to main/develop or release | Build & push image to GHCR |
| `deploy-staging` | push to `main` | Rolling update on staging K8s |
| `deploy-production` | GitHub release published | DB migrations + rolling update on prod K8s |

### Required GitHub Secrets

| Secret | Description |
|--------|-------------|
| `CODECOV_TOKEN` | Codecov upload token |
| `KUBE_CONFIG_STAGING` | base64-encoded kubeconfig for staging |
| `KUBE_CONFIG_PROD` | base64-encoded kubeconfig for production |
| `PROD_DATABASE_URL` | Production Postgres DSN |

---

## 9. Secrets Management

> [!WARNING]
> Never hard-code secrets in source code, Dockerfiles, or Kubernetes YAML manifests.

### Recommended Approaches

**Development**: `.env` file (git-ignored)

**Kubernetes**:
```bash
# Create from literal values
kubectl create secret generic flip-bills-secrets \
  --from-literal=JWT_SECRET="$(openssl rand -hex 32)" \
  --from-literal=PAYSTACK_SECRET_KEY="sk_live_..." \
  -n flip-bills
```

**Production**: Use a secrets manager (GCP Secret Manager, AWS Secrets Manager, HashiCorp Vault) with a Kubernetes External Secrets Operator.

### Rotating JWT Secrets

1. Generate a new secret: `openssl rand -hex 32`
2. Update the Kubernetes secret
3. Trigger a rolling restart: `kubectl rollout restart deployment/flipbills-api -n flip-bills`
4. All existing tokens will be invalidated — users must log in again

---

## 10. Rollback Procedures

### Kubernetes Rollback

```bash
# View rollout history
kubectl rollout history deployment/flipbills-api -n flip-bills

# Roll back to previous version
kubectl rollout undo deployment/flipbills-api -n flip-bills

# Roll back to a specific revision
kubectl rollout undo deployment/flipbills-api --to-revision=3 -n flip-bills

# Verify
kubectl rollout status deployment/flipbills-api -n flip-bills
```

### Database Rollback

```bash
# Rollback one migration step
migrate -path ./migrations -database "$DATABASE_URL" down 1

# Rollback to a specific version
migrate -path ./migrations -database "$DATABASE_URL" goto 11
```

> [!CAUTION]
> Database rollbacks that drop tables or columns cause data loss. Always take a backup before applying or reverting migrations in production.

---

## 11. Health Checks & Smoke Tests

### API Health Endpoint

```bash
curl https://api.flipbills.app/health
# Expected: 200 OK
# Body: {"status":"ok","version":"...","db":"connected","redis":"connected"}
```

### Post-Deployment Smoke Tests

```bash
# 1. Health check
STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://api.flipbills.app/health)
[ "$STATUS" = "200" ] && echo "✅ Health OK" || echo "❌ Health FAIL"

# 2. Auth endpoint reachable (should return 400 — validates routing)
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST https://api.flipbills.app/api/v1/auth/login)
[ "$STATUS" = "400" ] && echo "✅ Auth routing OK" || echo "❌ Auth routing FAIL"

# 3. Check Prometheus is scraping the API
curl -s http://prometheus:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job=="flip-bills-api") | .health'
# Expected: "up"
```

---

## 12. Troubleshooting

### API won't start

```bash
# Check logs
kubectl logs -l app=flipbills-api -n flip-bills --tail=100

# Common causes:
# - DATABASE_URL not set or unreachable → check pg_isready
# - JWT_SECRET too short → must be ≥32 characters
# - Port already in use → check $PORT env var
```

### Database connection refused

```bash
# Verify Postgres is running and healthy
docker compose ps postgres
docker compose exec postgres pg_isready -U flipbills

# Check pg_hba.conf — ensure the API's source IP is whitelisted
# In production, use connection pooling (PgBouncer) to avoid connection exhaustion
```

### RabbitMQ consumer not processing messages

```bash
# Check consumer logs
kubectl logs -l app=flipbills-worker -n flip-bills --tail=100

# Check queue status via management UI
# http://localhost:15672 → Queues tab → inspect queue depth and consumer count

# Restart consumers
kubectl rollout restart deployment/flipbills-worker -n flip-bills
```

### Migration failed — dirty state

```bash
# Force the version to the last known good state
migrate -path ./migrations -database "$DATABASE_URL" force <version>

# Then re-apply
migrate -path ./migrations -database "$DATABASE_URL" up
```

### High memory usage in Redis

```bash
# Check memory breakdown
docker compose exec redis redis-cli info memory

# Identify large keys
docker compose exec redis redis-cli --bigkeys

# If near maxmemory, increase limit in docker-compose.yml:
# command: redis-server --maxmemory 512mb --maxmemory-policy allkeys-lru
```

---

*For architecture details, see [ARCHITECTURE.md](./ARCHITECTURE.md).*
*For API reference, see the OpenAPI spec at `/api/v1/docs` (Swagger UI).*
