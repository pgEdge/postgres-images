-- app gets read access to the reference tables the extension's own
-- functions query (tiger.*, created at CREATE EXTENSION time).
--
-- The actual Census dataset a customer bulk-loads afterward is a
-- separate step this script cannot reach (it only runs once, at
-- CREATE EXTENSION time): loading it is ordinary CREATE TABLE/COPY,
-- nothing here requires supautils' gate or any elevated role, so
-- running that load connected as app lands it owned by app directly,
-- with no extra grant needed. See cnpg-cluster.md "Extensions".
--
-- Runs only when a role named "app" exists (see pg_cron's
-- after-create.sql for why): a no-op otherwise.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
    GRANT USAGE ON SCHEMA tiger TO app;
    GRANT SELECT ON ALL TABLES IN SCHEMA tiger TO app;
    ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA tiger
      GRANT SELECT ON TABLES TO app;
  END IF;
END;
$$;
