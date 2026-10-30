# LocalWhatsAppMCP landing

A small, left-aligned Next.js 16 App Router website. It runs independently of the native app and Go service. Downloads save the visitor's email to a PostgreSQL signup list first.

## Run locally

Use Node.js 22 or later. From the repository root:

```sh
cd landing
npm ci
cp .env.example .env.local
npm run dev
```

Open [localhost:3000](http://localhost:3000). To check and run the production build from `landing/`:

```sh
npm run typecheck
npm run build
npm start
```

Fonts use `next/font/google`: the build needs access to Google Fonts, then serves the downloaded fonts with the site.

Set `DATABASE_URL` in `.env.local` to the same server-only connection string used by the keyboard landing. This uses its existing `public.waitlist_signups` table; no table structure change is needed. The page and production build work without this setting, but signup submissions show a retryable error until storage is configured.

## Deploy to Vercel

Import this repository as a Vercel project. Set **Root Directory** to `landing`, choose the **Next.js** framework preset, and use Node.js 22 or later. Keep the preset's build and output settings.

Set Vercel's server-only `DATABASE_URL` to the same value as the keyboard landing before deploying. Preserve its direct PostgreSQL connection and TLS settings. Do not prefix this variable with `NEXT_PUBLIC_`; database credentials must stay on the server. For preview testing, use a separate test database with the same table structure.

If the shared connection uses the original insert-only `waitlist_writer`, run [db/append-source-permissions.sql](db/append-source-permissions.sql) as the table owner before deploying either landing. It adds `SELECT (email, source)`, `UPDATE (source)`, and the corresponding row-level security policies, retaining the existing insert permissions. This script is safe to rerun and is only needed once for the shared database. A connection that already has these permissions does not need it.

Open Graph and X/Twitter previews use the existing `/setup.gif`. Absolute image URLs use `SITE_URL` when set (a full public origin such as `https://your-domain.com`), otherwise Vercel's production domain, its deployment domain, or localhost during local development. Set `SITE_URL` at build time to use a custom domain. Preview services decide whether to animate the GIF or display a still frame.

## Signup list

[app/api/signups/route.ts](app/api/signups/route.ts) uses the same storage contract as the keyboard landing: a direct PostgreSQL connection per submission and an atomic `INSERT ... ON CONFLICT (email) DO UPDATE`. Emails are trimmed and lowercased. This English page always supplies `locale = 'en'` and `source = 'landing_localwhatsappmcp'` on the server; request fields cannot override them. No emails are sent.

The shared table is unique by email. For an existing address, the handler appends its source only if that complete comma-separated label is missing: `landing` becomes `landing,landing_localwhatsappmcp`. Repeated or simultaneous submissions do not add duplicate sources. The existing ID, locale and signup timestamp are preserved. The keyboard landing follows the same behavior, so attribution accumulates whichever page is visited first. [db/schema.sql](db/schema.sql) documents the compatible table structure for local tests.

The browser starts the DMG download only after the database confirms the write. If storage is unavailable, it preserves the entered email and offers a retry. The standard HTML form also supports a POST-and-redirect download without JavaScript. Email addresses are sent in the request body, never the download URL. There is no public endpoint for reading the signup list.

Read the list in your database provider's SQL editor:

```sql
select email, locale, source, created_at
from public.waitlist_signups
where 'landing_localwhatsappmcp' = any(string_to_array(source, ','))
order by created_at desc;
```

## Verify signup storage

Run `npm test` from `landing/` with PostgreSQL's `initdb` and `pg_ctl` available on `PATH`. The test builds the app, creates a disposable local database and server, and checks validation, source appending, repeated and simultaneous signups, unchanged existing metadata, the standard form redirect, and database failures. Requests use a restricted writer with row-level security and the permission update above; it cannot change other existing columns. The test uses synthetic emails, ignores any configured production `DATABASE_URL`, and removes its temporary database when finished.

## Content and downloads

- [app/page.tsx](app/page.tsx) contains the page and setup GIF. Preserve the requested copy, left alignment and simple spacing when editing.
- [app/globals.css](app/globals.css) and [app/fonts.ts](app/fonts.ts) hold the style primitives: white background, Manrope headings and Inter body text.
- [app/site.ts](app/site.ts) holds the description, repository link and direct DMG URL, currently pinned to release `v1.1.1`. Update the URL when publishing a new app release.
- [app/DownloadForm.tsx](app/DownloadForm.tsx) submits the email to the signup endpoint and starts the direct DMG download after a successful save.
- [public/setup.gif](public/setup.gif) is a copy of [docs/images/setup.gif](../docs/images/setup.gif), a setup preview using sample data. When the source changes, refresh the copy from the repository root:

```sh
cp docs/images/setup.gif landing/public/setup.gif
```
