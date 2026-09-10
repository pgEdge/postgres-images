-- Restricts CREATE EXTENSION lolor to the privileged role, and refuses
-- it unless its own required dependency is already installed in the
-- same database. lolor's install script renames core Postgres
-- large-object functions (pg_catalog.lo_open among them) in place to
-- intercept large-object operations, and that rename takes effect
-- immediately: without the dependency in place, the next
-- lo_create()/lo_open() call anywhere in the database, by any role,
-- then fails, with no self-service recovery, only genuine superuser
-- can undo it (lolor's own disable routine needs to own
-- pg_catalog.lo_open, which no non-superuser role does).
--
-- The role check here (not just this extension's own control-file
-- trust flag) is what makes this reliable: supautils.privileged_role
-- only ever benefited from being the sole role whose session loads
-- supautils, an emergent property, not an enforced one. Checking
-- session_user directly closes that gap regardless of how supautils
-- is loaded.
DO $$
BEGIN
  IF NOT pg_has_role(session_user, current_setting('supautils.privileged_role'), 'MEMBER') THEN
    RAISE EXCEPTION 'only % can install lolor, connected as %', current_setting('supautils.privileged_role'), session_user;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'spock') THEN
    RAISE EXCEPTION 'lolor requires spock to already be installed in this database: installing lolor without it breaks large-object support for every role, with no self-service recovery';
  END IF;
END;
$$;
