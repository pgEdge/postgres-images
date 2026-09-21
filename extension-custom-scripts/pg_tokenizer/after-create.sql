-- Read access to admin-configured tokenizer definitions
-- (tokenizer_catalog.*), not write: whoever configures tokenizers
-- stays a separate, more privileged concern. Scoped to this one
-- schema, not a database-wide default, so a future gated extension's
-- own schema isn't exposed without its own deliberate grant here.
--
-- Also granted to PUBLIC: pg_database_owner's own grant carries no
-- GRANT OPTION, so there is no way to pass it on to another role
-- afterward, and the attempt is a silent no-op, not an error. The
-- write side stays restricted to pg_database_owner only.
GRANT USAGE ON SCHEMA tokenizer_catalog TO pg_database_owner, PUBLIC;
GRANT SELECT ON ALL TABLES IN SCHEMA tokenizer_catalog TO pg_database_owner, PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tokenizer_catalog
  GRANT SELECT ON TABLES TO pg_database_owner, PUBLIC;

-- Schema USAGE does not just unlock reading the tables above, it
-- makes every function in this schema callable by any role, since
-- Postgres grants EXECUTE on new functions to PUBLIC by default. Most
-- of what lives here manages tokenizer/model configuration
-- (create_*, drop_*, add_preload_model, and friends), none of it
-- SECURITY DEFINER, so the table writes those functions attempt are
-- still refused on ACL, confirmed directly. But create_huggingface_model
-- and create_lindera_model run real work, parsing a config and
-- attempting to load a model, before any permission check fires, and
-- a role with only schema USAGE can reach them now. Revokes EXECUTE
-- from PUBLIC on everything in the schema, keeps it for
-- pg_database_owner explicitly rather than leaving it dependent on
-- the PUBLIC default just revoked, then re-grants PUBLIC only the
-- three functions the read-only use case actually needs: tokenize()
-- and apply_text_analyzer() to process text against an existing
-- configuration, and list_preload_models() to see what is available.
-- Configuring a new tokenizer, model, or analyzer stays a privileged
-- operation.
REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA tokenizer_catalog FROM PUBLIC;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA tokenizer_catalog TO pg_database_owner;
GRANT EXECUTE ON FUNCTION tokenizer_catalog.tokenize(text, text) TO PUBLIC;
GRANT EXECUTE ON FUNCTION tokenizer_catalog.apply_text_analyzer(text, text) TO PUBLIC;
GRANT EXECUTE ON FUNCTION tokenizer_catalog.list_preload_models() TO PUBLIC;

-- Postgres seeds every new function with PUBLIC execute regardless of
-- ALTER DEFAULT PRIVILEGES, which cannot override that built-in
-- default, only add to it. A function an extension upgrade adds later
-- is not covered by the REVOKE above, since that only reaches
-- functions that already exist when this script runs, so anything
-- added afterward is locked down here instead: any CREATE or REPLACE
-- of a function in this schema drops PUBLIC execute and grants
-- pg_database_owner, the same posture the REVOKE above establishes at
-- install time. This also re-locks the three functions granted to
-- PUBLIC above if a future version of the extension replaces them,
-- so reopening those after an upgrade is a deliberate step, not
-- something that happens on its own. supautils skips an event trigger
-- for a superuser that does not own its function, so this does not
-- fire for a function added by a superuser other than the one that
-- ran this script.
CREATE OR REPLACE FUNCTION tokenizer_catalog.lock_down_new_function()
RETURNS event_trigger
LANGUAGE plpgsql
AS $lockdown$
DECLARE
  obj record;
BEGIN
  FOR obj IN SELECT * FROM pg_event_trigger_ddl_commands() LOOP
    IF obj.object_type = 'function' AND obj.schema_name = 'tokenizer_catalog' THEN
      EXECUTE format('REVOKE EXECUTE ON FUNCTION %s FROM PUBLIC', obj.object_identity);
      EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO pg_database_owner', obj.object_identity);
    END IF;
  END LOOP;
END;
$lockdown$;

DROP EVENT TRIGGER IF EXISTS tokenizer_catalog_lock_new_functions;
CREATE EVENT TRIGGER tokenizer_catalog_lock_new_functions
  ON ddl_command_end
  WHEN TAG IN ('CREATE FUNCTION')
  EXECUTE FUNCTION tokenizer_catalog.lock_down_new_function();
