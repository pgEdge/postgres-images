-- The database's own owner gets read access to admin-configured
-- tokenizer definitions (tokenizer_catalog.*), not write: whoever
-- configures tokenizers stays a separate, more privileged concern.
-- Scoped to this one schema, not a database-wide default, so a future
-- gated extension's own schema isn't exposed without its own
-- deliberate grant here.
--
-- Granted to pg_database_owner rather than a hardcoded role name, so
-- this keeps working if the database is later reassigned to a
-- different owner. See
-- https://www.postgresql.org/docs/current/predefined-roles.html.
GRANT USAGE ON SCHEMA tokenizer_catalog TO pg_database_owner;
GRANT SELECT ON ALL TABLES IN SCHEMA tokenizer_catalog TO pg_database_owner;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tokenizer_catalog
  GRANT SELECT ON TABLES TO pg_database_owner;
