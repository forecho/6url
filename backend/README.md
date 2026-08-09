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

Create a local administrator token after migration and after configuring `ADMIN_TOKEN_PEPPER`:

```bash
go run ./cmd/6url-admin token create \
  --name local-ai \
  --scopes catalog:read,content:write,content:publish,assets:write,submissions:review,ad_applications:review,placements:write,ads:write
```

The raw token is printed once. Store it outside the repository.

Import the current JSON catalog as Chinese drafts with active editorial placements:

```bash
go run ./cmd/6url-admin import-json --resources ../data/resources.json
```

The import is idempotent. Imported content remains draft until it passes the publication checks; the current Next.js JSON catalog remains the public fallback during migration.
