# Cherries

Cherries is an open source site navigation system.

## Getting Started

First, run the development server:

```bash
npm run dev
# or
yarn dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

## Make yourself site

### [Fork this repository](https://github.com/forecho/cherries/fork)

### Configure your site

Edit `data/site.json` and `data/resources.json` to configure the legacy JSON source.

The database migration keeps this JSON catalog as the default public source. Set `SIXURL_CATALOG_SOURCE=database` and `SIXURL_API_URL` after imported content has passed review and publication.

Backend setup and administrator token commands are documented in `backend/README.md`.

For a fresh backend database, run `6url-admin migrate` before creating an administrator token or importing the JSON catalog. Real environment files and credentials stay outside the repository; the committed `.env.example` files list every supported variable.

### Deploy on Vercel

The easiest way to deploy your Next.js app is to use the [Vercel Platform](https://vercel.com/new?utm_medium=default-template&filter=next.js&utm_source=create-next-app&utm_campaign=create-next-app-readme) from the creators of Next.js.

Check out our [Next.js deployment documentation](https://nextjs.org/docs/deployment) for more details.
