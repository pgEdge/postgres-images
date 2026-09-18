-- supautils runs this immediately after CREATE EXTENSION pg_cron, as the
-- same superuser session used to install the extension (see
-- supautils.superuser). pg_cron's install script creates cron.job and
-- cron.job_run_details owned by that superuser, leaving the current
-- database's own owner with no path to manage its own scheduled jobs
-- or review their run history.
--
-- Granted to pg_database_owner rather than a hardcoded role name:
-- Postgres automatically maintains membership in this predefined role to
-- match whoever currently owns the database pg_cron was installed in, so
-- this keeps working correctly if that database is later reassigned to a
-- different owner, and needs no assumption about what that owner is
-- named. See https://www.postgresql.org/docs/current/predefined-roles.html.
--
-- SELECT only, ownership stays with the installing superuser:
-- cron.schedule() and cron.unschedule() are not SECURITY DEFINER, they
-- run as the caller, but they write to cron.job through pg_cron's own
-- internal C code, not through a normal caller-privileged INSERT or
-- UPDATE. Confirmed directly: a role with only this SELECT grant can
-- schedule, list, and unschedule its own jobs through those functions,
-- and a raw INSERT or UPDATE against cron.job as that role is refused
-- outright, permission denied, with no ownership or row-level-security
-- involved at all. Nothing here ever needs ownership to work.
GRANT USAGE ON SCHEMA cron TO pg_database_owner;
GRANT SELECT ON cron.job TO pg_database_owner;
GRANT SELECT ON cron.job_run_details TO pg_database_owner;
