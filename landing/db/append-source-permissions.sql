-- Run as the table owner before deploying either landing with source appending.
-- The existing waitlist_writer INSERT grant and policy remain in place.
BEGIN;

GRANT SELECT (email, source), UPDATE (source)
    ON public.waitlist_signups TO waitlist_writer;

DROP POLICY IF EXISTS waitlist_writer_select ON public.waitlist_signups;
CREATE POLICY waitlist_writer_select ON public.waitlist_signups
    FOR SELECT TO waitlist_writer USING (true);

DROP POLICY IF EXISTS waitlist_writer_update ON public.waitlist_signups;
CREATE POLICY waitlist_writer_update ON public.waitlist_signups
    FOR UPDATE TO waitlist_writer USING (true) WITH CHECK (true);

COMMIT;
