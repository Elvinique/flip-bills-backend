# Travel Partner Validation Notes

## Public Documentation Check

Public search did not surface official GIGM or ABC Transport partner API documentation for route search, seat hold, booking confirmation, or cancellation.

What is publicly visible:

- GIGM has a public booking surface for bus terminals and seat booking, but no public API contract was found.
- ABC Transport has public online booking information and route listings, but no public API contract was found.

Because the partner API contracts appear private, the backend now treats the bus partner contract as configurable instead of hardcoding guessed endpoints.

## Configurable Contract Fields

Set these per partner once sandbox documentation is available:

```env
GIGM_BASE_URL=
GIGM_SEARCH_PATH=
GIGM_HOLD_PATH=
GIGM_CONFIRM_PATH=
GIGM_CANCEL_PATH=
GIGM_AUTH_HEADER=
GIGM_AUTH_SCHEME=
GIGM_SECONDARY_AUTH_HEADER=

ABC_BASE_URL=
ABC_SEARCH_PATH=
ABC_HOLD_PATH=
ABC_CONFIRM_PATH=
ABC_CANCEL_PATH=
ABC_AUTH_HEADER=
ABC_AUTH_SCHEME=
ABC_SECONDARY_AUTH_HEADER=
```

Defaults remain:

```text
SEARCH_PATH=/trips/search
HOLD_PATH=/seats/hold
CONFIRM_PATH=/bookings/confirm
CANCEL_PATH=/bookings/cancel
AUTH_HEADER=Authorization
AUTH_SCHEME=Bearer
SECONDARY_AUTH_HEADER=X-API-Key
```

Set `*_AUTH_SCHEME` to an empty value if a partner expects the raw API key instead of `Bearer <key>`.
Set `*_SECONDARY_AUTH_HEADER` to an empty value if the partner rejects extra API-key headers.

## Opt-In Sandbox Validation

The live contract tests are skipped by default. Run them only against a sandbox or a partner-approved test environment.

```bash
GIGM_VALIDATE_SANDBOX=true \
GIGM_API_KEY=... \
GIGM_BASE_URL=... \
GIGM_SANDBOX_ORIGIN=Lagos \
GIGM_SANDBOX_DESTINATION=Abuja \
GIGM_SANDBOX_DEPARTURE_DATE=2026-06-01 \
GOCACHE=/tmp/flip-bills-gocache go test ./internal/services/travel/operators -run TestGIGMSandboxContract -v
```

```bash
ABC_VALIDATE_SANDBOX=true \
ABC_API_KEY=... \
ABC_BASE_URL=... \
ABC_SANDBOX_ORIGIN=Lagos \
ABC_SANDBOX_DESTINATION=Abuja \
ABC_SANDBOX_DEPARTURE_DATE=2026-06-01 \
GOCACHE=/tmp/flip-bills-gocache go test ./internal/services/travel/operators -run TestABCSandboxContract -v
```

The sandbox tests validate:

- Search endpoint is reachable.
- Auth headers are accepted.
- Response schema can map into `BusSearchResult`.
- At least one result includes `vehicle_ref` and `price_kobo`.
- Seat hold returns a hold reference.
- Confirm returns a ticket/reference.
- Cancel succeeds for the confirmed sandbox ticket.

## Partner Contract Details Needed

Request the following from GIGM and ABC:

- Sandbox and production base URLs.
- Search, hold, confirm, cancel endpoint paths and HTTP methods.
- Required auth header names and whether values are `Bearer <key>` or raw keys.
- Required request fields for route search, hold, confirm, cancel.
- Response examples for successful and failed calls.
- Seat-map schema and unavailable-seat representation.
- Cancellation behavior for sandbox confirmations.
- Rate limits and idempotency requirements.
