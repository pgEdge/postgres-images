-- app needs USAGE on bm25_catalog to declare a column of its
-- bm25vector type and call its functions (search_bm25query,
-- to_bm25query); Postgres already grants EXECUTE on new functions to
-- PUBLIC by default, so no separate function grant is needed. The
-- schema holds only the type and its support functions, no tables.
--
-- Runs only when a role named "app" exists (see pg_cron's
-- after-create.sql for why): a no-op otherwise.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
    GRANT USAGE ON SCHEMA bm25_catalog TO app;
  END IF;
END;
$$;
