-- supautils runs this immediately after CREATE EXTENSION pg_cron, as the
-- same superuser session used to install the extension (see
-- supautils.superuser). pg_cron's install script creates cron.job and
-- cron.job_run_details owned by that superuser.
--
-- This image is not exclusive to any one deployment's role model, so the
-- handoff below only runs when a role named "app" actually exists,
-- rather than assuming every consumer of this image has one. Where it
-- does (e.g. drydock's tenant databases), app ends up owning its own
-- scheduled jobs outright instead of only being able to read them,
-- with the schema-level USAGE that ownership alone doesn't carry.
-- Where it doesn't, this is a no-op: CREATE EXTENSION pg_cron still
-- succeeds and cron.job/cron.job_run_details stay owned by the
-- superuser, exactly as they do today.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
    GRANT USAGE ON SCHEMA cron TO app;
    ALTER TABLE cron.job OWNER TO app;
    ALTER TABLE cron.job_run_details OWNER TO app;
  END IF;
END;
$$;
