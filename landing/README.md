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

Set `DATABASE_URL` in `.env.local` to the same server-only connection string used by the keyboard landing. This uses its existing `public.waitlist_signups` table and insert-only `waitlist_writer` user; no new table or migration is needed. The page and production build work without this setting, but signup submissions show a retryable error until storage is configured.

## Deploy to Vercel

Import this repository as a Vercel project. Set **Root Directory** to `landing`, choose the **Next.js** framework preset, and use Node.js 22 or later. Keep the preset's build and output settings.

Set Vercel's server-only `DATABASE_URL` to the same value as the keyboard landing before deploying. Preserve its direct PostgreSQL connection and TLS settings. Do not prefix this variable with `NEXT_PUBLIC_`; database credentials must stay on the server. For preview testing, use a separate test database with the same table structure.

## Signup list

[app/api/signups/route.ts](app/api/signups/route.ts) uses the same storage contract as the keyboard landing: a direct PostgreSQL connection per submission and `INSERT INTO public.waitlist_signups (email, locale, source) ... ON CONFLICT DO NOTHING`. Emails are trimmed and lowercased. This English page always writes `locale = 'en'` and `source = 'landing_localwhatsappmcp'` on the server; request fields cannot override them. No emails are sent.

The shared table is unique by email. An address already registered through either landing succeeds without inserting a duplicate or changing the existing row's source, locale or timestamp, matching the keyboard landing's behavior. [db/schema.sql](db/schema.sql) documents the compatible table structure for local tests; production reuses the existing table, grants and row-level security policy.

The browser starts the DMG download only after the database confirms the write. If storage is unavailable, it preserves the entered email and offers a retry. The standard HTML form also supports a POST-and-redirect download without JavaScript. Email addresses are sent in the request body, never the download URL. There is no public endpoint for reading the signup list.

Read the list in your database provider's SQL editor:

```sql
select email, locale, source, created_at
from public.waitlist_signups
where source = 'landing_localwhatsappmcp'
order by created_at desc;
```

## Verify signup storage

Run `npm test` from `landing/` with PostgreSQL's `initdb` and `pg_ctl` available on `PATH`. The test builds the app, creates a disposable local database and server, and checks validation, source attribution, shared-table duplicates, the standard form redirect, and database failures. Requests use an insert-only role with row-level security, matching the existing table's access contract. The test uses synthetic emails, ignores any configured production `DATABASE_URL`, and removes its temporary database when finished.

## Content and downloads

- [app/page.tsx](app/page.tsx) contains the page and setup GIF. Preserve the requested copy, left alignment and simple spacing when editing.
- [app/globals.css](app/globals.css) and [app/fonts.ts](app/fonts.ts) hold the style primitives: white background, Manrope headings and Inter body text.
- [app/site.ts](app/site.ts) holds the description, repository link and direct DMG URL, currently pinned to release `v1.1.1`. Update the URL when publishing a new app release.
- [app/DownloadForm.tsx](app/DownloadForm.tsx) submits the email to the signup endpoint and starts the direct DMG download after a successful save.
- [public/setup.gif](public/setup.gif) is a copy of [docs/images/setup.gif](../docs/images/setup.gif), a setup preview using sample data. When the source changes, refresh the copy from the repository root:

```sh
cp docs/images/setup.gif landing/public/setup.gif
```
