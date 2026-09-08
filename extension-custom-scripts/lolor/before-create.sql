-- Refuses CREATE EXTENSION lolor unless spock is already installed in
-- the same database, for every role, no exceptions. lolor's install
-- script renames core Postgres large-object functions
-- (pg_catalog.lo_open among them) in place to intercept large-object
-- operations, and that rename takes effect immediately regardless of
-- whether lolor.* GUCs are configured: without spock, the next
-- lo_create()/lo_open() call anywhere in the database, by any role,
-- then fails, with no self-service recovery, only genuine superuser
-- can undo it (lolor's own disable routine needs to own
-- pg_catalog.lo_open, which no non-superuser role does).
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'spock') THEN
    RAISE EXCEPTION 'lolor requires spock to already be installed in this database: installing lolor without spock breaks large-object support for every role, with no self-service recovery';
  END IF;
END;
$$;
