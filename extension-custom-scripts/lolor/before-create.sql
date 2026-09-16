-- Refuses CREATE EXTENSION lolor unless spock is already installed in
-- the same database, for every role whose session has supautils
-- loaded at all (every role, no exceptions, in the common deployment
-- where supautils loads cluster-wide via shared_preload_libraries; a
-- deployment that loads it only into specific sessions, the way it
-- scopes the extension gate itself, gets this check only in those
-- sessions). lolor's install script renames core Postgres large-object
-- functions (pg_catalog.lo_open among them) in place to intercept
-- large-object operations, and that rename takes effect immediately
-- regardless of whether lolor.* GUCs are configured: without spock,
-- the next lo_create()/lo_open() call anywhere in the database, by any
-- role, then fails, with no self-service recovery, only genuine
-- superuser can undo it (lolor's own disable routine needs to own
-- pg_catalog.lo_open, which no non-superuser role does).
--
-- This fires regardless of whether lolor is on
-- supautils.privileged_extensions: confirmed directly, supautils runs
-- a before-create.sql for any extension that has one, for any session
-- with the hook active, independent of the allowlist. The allowlist
-- only controls whether the extension's actual install gets a
-- superuser switch; lolor ships trusted=true, so it never needed that
-- switch to install for a role with plain CREATE privilege in the
-- first place. This is the only thing standing between such a role
-- and lolor's destructive install if the caller hasn't set up a
-- separate, deployment-specific way to block it.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_extension WHERE extname = 'spock') THEN
    RAISE EXCEPTION 'lolor requires spock to already be installed in this database: installing lolor without spock breaks large-object support for every role, with no self-service recovery';
  END IF;
END;
$$;
