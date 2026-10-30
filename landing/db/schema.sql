-- Compatible with the shared keyboard landing table; no migration is needed
-- when public.waitlist_signups already exists. Used by local integration tests.
CREATE TABLE IF NOT EXISTS public.waitlist_signups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL UNIQUE,
    locale text NOT NULL DEFAULT 'en',
    source text NOT NULL DEFAULT 'landing',
    created_at timestamptz NOT NULL DEFAULT now()
);
