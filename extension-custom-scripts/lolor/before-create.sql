-- Refuses every CREATE EXTENSION lolor until spock is deployed, for
-- every role, no exceptions. lolor's install script renames core
-- Postgres large-object functions (pg_catalog.lo_open among them) in
-- place to intercept large-object operations, and that rename takes
-- effect immediately whether or not spock or lolor.* GUCs are
-- configured: the next lo_create()/lo_open() call anywhere in the
-- database, by any role, then fails, with no self-service recovery.
-- Only genuine superuser can undo it (lolor's own disable routine
-- needs to own pg_catalog.lo_open, which no non-superuser role does).
DO $$
BEGIN
  RAISE EXCEPTION 'lolor is blocked: it needs spock, not yet deployed, and installing it alone can break large-object support with no self-service recovery';
END;
$$;
