-- The database's owner can schedule, list, change, and remove jobs,
-- and read their run history. It cannot write cron.job or
-- cron.job_run_details directly: schedule() and unschedule() write
-- those tables through pg_cron's own code, so read access to them is
-- all the owner needs. Calling schedule() again with an existing job
-- name changes that job's schedule and command in place.
--
-- alter_job() and schedule_in_database() stay revoked, as pg_cron
-- leaves them at install time. Both can point a job at another
-- database the owner has CONNECT on, and pg_cron's workers connect
-- there without going through pg_hba.conf. Keeping them revoked means
-- a job always runs in the database that scheduled it. The cost is
-- that the owner cannot pause a job; it unschedules it instead.
GRANT USAGE ON SCHEMA cron TO pg_database_owner;
GRANT SELECT ON cron.job TO pg_database_owner;
GRANT SELECT ON cron.job_run_details TO pg_database_owner;
