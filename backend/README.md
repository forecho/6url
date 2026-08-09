# 6URL API

Go API for the 6URL directory, submission, advertising and administration workflows.

## Local setup

```bash
cp .env.example .env
make ent-gen
make run
```

The service requires MySQL. Apply the schema and seed the four supported locales before starting the API:

```bash
go run ./cmd/6url-admin migrate
```

The API process never executes DDL during startup. Run the migration command once for each reviewed release before rolling out API instances.
The same command seeds the single USD paid-listing plan and Sponsor Ad offer from the Stripe amount, duration and Price ID environment variables.

Create a local administrator token after migration and after configuring `ADMIN_TOKEN_PEPPER`:

```bash
go run ./cmd/6url-admin token create \
  --name local-ai \
  --scopes catalog:read,content:write,content:publish,assets:write,submissions:review,ad_applications:review,placements:write,ads:write
```

The raw token is printed once. Store it outside the repository.

API contracts are available after startup:

```text
GET /api/v1/openapi.json
GET /admin/v1/openapi.json
```

Public applications use Cloudflare Turnstile and Redis rate limiting. Status links carry their raw token in the URL fragment, exchange it for a 30-minute HttpOnly cookie, and require Origin plus `X-CSRF-Token` on status mutations and Checkout creation. Run the API process continuously so its durable notification worker can deliver UnoSend or Sendflare email and Slack outbox rows.

Published submissions enqueue signed frontend revalidation jobs. Configure the same secret as backend `REVALIDATION_SECRET` and frontend `SIXURL_REVALIDATION_SECRET`; the backend worker calls `REVALIDATION_WEBHOOK_URL` until Next.js confirms the affected paths were revalidated.

## Stripe webhook isolation

The Stripe account may serve multiple products. Create a dedicated webhook endpoint for 6URL, give it its own signing secret, and subscribe it only to these events:

```text
checkout.session.completed
checkout.session.async_payment_succeeded
checkout.session.async_payment_failed
checkout.session.expired
refund.created
refund.updated
refund.failed
charge.refund.updated
```

6URL writes `sixurl_app=6url.net` and `sixurl_order_id` into Checkout Session, PaymentIntent and Refund metadata. The webhook also verifies the local Checkout Session or PaymentIntent before changing an order. Events owned by another product and event types outside the list receive HTTP 200 with `ignored=true` and are excluded from `payment_events`. Checkout Sessions created before this metadata was introduced remain compatible through their locally stored Stripe Session ID.

Import the current JSON catalog as Chinese drafts with active editorial placements:

```bash
go run ./cmd/6url-admin import-json --resources ../data/resources.json
```

The import is idempotent. Imported content remains draft until it passes the publication checks; the current Next.js JSON catalog remains the public fallback during migration.
