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

-- Row level security does not apply to a table's own owner by default,
-- only to everyone else, and the two ALTER TABLE ... OWNER statements
-- above just made pg_database_owner (and so whichever role owns this
-- database) the owner of both tables. Without this, that owner could
-- insert or update a row with username set to any role, including
-- postgres, bypassing cron.job's own "username = CURRENT_USER" policy
-- entirely. With cron.use_background_workers on, pg_cron's launcher
-- then tries to execute that row in-process, which crashes the whole
-- instance rather than raising a clean permission error. Forcing row
-- level security here closes that: the owner is held to the same
-- policy as anyone else, while cron.schedule()/cron.unschedule() keep
-- working normally, since a role scheduling its own job already sets
-- username to itself.
ALTER TABLE cron.job FORCE ROW LEVEL SECURITY;
ALTER TABLE cron.job_run_details FORCE ROW LEVEL SECURITY;
