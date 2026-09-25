-- Every role can read the us_lex, us_gaz, and us_rules reference
-- tables that standardize_address() takes as arguments. No role but
-- the installing superuser can modify them.
--
-- The extension can be installed into any schema, so the schema is
-- read from pg_extension rather than assumed to be public. supautils'
-- @extschema@ cannot stand in for the lookup: it is set only when
-- CREATE EXTENSION names a schema, and is NULL otherwise. USAGE on
-- that schema is granted too, since table access alone does not reach
-- a table in a schema the role cannot use.
DO $$
DECLARE ext_schema name;
BEGIN
  SELECT n.nspname INTO ext_schema
  FROM pg_catalog.pg_extension e
  JOIN pg_catalog.pg_namespace n ON n.oid = e.extnamespace
  WHERE e.extname = 'address_standardizer_data_us';

  EXECUTE format(
    'GRANT USAGE ON SCHEMA %I TO pg_database_owner, PUBLIC',
    ext_schema
  );
  EXECUTE format(
    'GRANT SELECT ON TABLE %I.us_lex, %I.us_gaz, %I.us_rules TO pg_database_owner, PUBLIC',
    ext_schema, ext_schema, ext_schema
  );
END
$$;
