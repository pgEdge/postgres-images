-- app gets read access to admin-configured tokenizer definitions
-- (tokenizer_catalog.*), not write: admin remains the role that
-- configures tokenizers. Scoped to this one schema, not a
-- database-wide default, so a future admin-gated extension's own
-- schema isn't exposed to app without its own deliberate grant here.
--
-- Runs only when a role named "app" exists (see pg_cron's
-- after-create.sql for why): a no-op otherwise.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
    GRANT USAGE ON SCHEMA tokenizer_catalog TO app;
    GRANT SELECT ON ALL TABLES IN SCHEMA tokenizer_catalog TO app;
    ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA tokenizer_catalog
      GRANT SELECT ON TABLES TO app;
  END IF;
END;
$$;
