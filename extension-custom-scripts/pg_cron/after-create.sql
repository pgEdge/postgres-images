-- supautils runs this immediately after CREATE EXTENSION pg_cron, as the
-- same superuser session used to install the extension (see
-- supautils.superuser). pg_cron's install script creates cron.job and
-- cron.job_run_details owned by that superuser, leaving the current
-- database's own owner with no path to manage its own scheduled jobs.
--
-- Granted to pg_database_owner rather than a hardcoded role name:
-- Postgres automatically maintains membership in this predefined role to
-- match whoever currently owns the database pg_cron was installed in, so
-- this keeps working correctly if that database is later reassigned to a
-- different owner, and needs no assumption about what that owner is
-- named. See https://www.postgresql.org/docs/current/predefined-roles.html.
GRANT USAGE ON SCHEMA cron TO pg_database_owner;
ALTER TABLE cron.job OWNER TO pg_database_owner;
ALTER TABLE cron.job_run_details OWNER TO pg_database_owner;
