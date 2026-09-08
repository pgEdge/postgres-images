-- us_lex/us_gaz/us_rules land in public, owned by postgres. This
-- extension has no dedicated schema of its own. Grants read access to
-- exactly those three tables rather than a schema-wide default: app
-- (the tenant database owner) already owns the public schema itself,
-- so a schema-wide grant here would be redundant with that ownership.
--
-- Runs only when a role named "app" exists (see pg_cron's
-- after-create.sql for why): a no-op otherwise.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
    GRANT SELECT ON TABLE public.us_lex, public.us_gaz, public.us_rules TO app;
  END IF;
END;
$$;
