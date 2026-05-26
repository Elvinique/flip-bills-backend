# Amadeus Sandbox Validation

## Should We Sign Up?

Yes. To validate Phase 2 flight integration against the real Amadeus sandbox, create an Amadeus for Developers account and generate Self-Service API credentials.

Official docs:

- API keys: https://developers.amadeus.com/self-service/apis-docs/guides/developer-guides/API-Keys/
- Quick start: https://developers.amadeus.com/self-service/apis-docs/guides/developer-guides/quick-start/
- Test data: https://developers.amadeus.com/self-service/apis-docs/guides/developer-guides/test-data/
- Authorization: https://developers.amadeus.com/self-service/apis-docs/guides/authorization-262

The Amadeus test environment uses:

```text
https://test.api.amadeus.com
```

## Required Values

Add these to `.env`:

```env
AMADEUS_CLIENT_ID=your-api-key
AMADEUS_CLIENT_SECRET=your-api-secret
AMADEUS_BASE_URL=https://test.api.amadeus.com
```

For validation tests:

```env
AMADEUS_VALIDATE_SANDBOX=true
AMADEUS_SANDBOX_ORIGIN=LOS
AMADEUS_SANDBOX_DESTINATION=ABV
AMADEUS_SANDBOX_DEPARTURE_DATE=YYYY-MM-DD
```

Use a date and route from Amadeus test-data guidance. The test environment has limited/cached data, so not every route/date will return offers.

## Run Search + Pricing Validation

```bash
AMADEUS_VALIDATE_SANDBOX=true \
AMADEUS_CLIENT_ID=... \
AMADEUS_CLIENT_SECRET=... \
AMADEUS_BASE_URL=https://test.api.amadeus.com \
AMADEUS_SANDBOX_ORIGIN=LOS \
AMADEUS_SANDBOX_DESTINATION=ABV \
AMADEUS_SANDBOX_DEPARTURE_DATE=2026-06-01 \
GOCACHE=/tmp/flip-bills-gocache go test ./internal/services/travel/operators -run TestAmadeusSandboxContract -v
```

## Optional Booking Validation

Only enable this when you intentionally want to create a test flight order:

```bash
AMADEUS_VALIDATE_BOOKING=true
```

Then rerun the sandbox test above.

## What The Test Validates

- OAuth client credentials grant succeeds.
- Flight search returns at least one mappable offer.
- Offer includes `gds_ref` and a valid price.
- Flight offer pricing confirms the selected offer.
- Optional booking creates an Amadeus flight order.
