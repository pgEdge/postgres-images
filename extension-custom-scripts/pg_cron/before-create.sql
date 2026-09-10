-- Restricts CREATE EXTENSION for this privileged extension to
-- supautils.privileged_role. Checking session_user directly (not the
-- role supautils elevates the session to internally) is what makes
-- this hold regardless of which sessions have supautils loaded:
-- session_user is the role that actually authenticated, unaffected by
-- supautils switching the acting role to install the extension.

DO $$
BEGIN
  IF NOT pg_has_role(session_user, current_setting('supautils.privileged_role'), 'MEMBER') THEN
    RAISE EXCEPTION 'only % can install this extension, connected as %', current_setting('supautils.privileged_role'), session_user;
  END IF;
END;
$$;
