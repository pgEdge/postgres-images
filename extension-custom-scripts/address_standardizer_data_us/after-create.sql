-- us_lex/us_gaz/us_rules land wherever the extension itself was
-- installed, owned by the supautils superuser. Unlike the other five
-- scripts in this directory, this extension is relocatable
-- (control file has no fixed schema), so a caller can run
-- CREATE EXTENSION address_standardizer_data_us SCHEMA gis and these
-- three tables land in gis, not public. A hardcoded public.us_lex
-- here would silently fail against relations that don't exist in
-- that case, which is why this looks the schema up at runtime rather
-- than assuming it.
--
-- Looked up via pg_extension.extnamespace rather than supautils' own
-- @extschema@ substitution: that token is only populated when the
-- caller's CREATE EXTENSION included an explicit SCHEMA clause, and
-- is otherwise substituted as SQL NULL, which is the common case
-- (no explicit SCHEMA at all). pg_extension.extnamespace is populated
-- unconditionally, by Postgres itself, once the extension exists, so
-- it covers both cases with the same query.
--
-- Granted to pg_database_owner rather than a hardcoded role name, so
-- this keeps working if the database is later reassigned to a
-- different owner. See
-- https://www.postgresql.org/docs/current/predefined-roles.html.
--
-- Also grants USAGE on the schema itself, not just SELECT on the
-- tables: table-level SELECT alone is not enough to query a table
-- outside the search path, Postgres separately checks USAGE on the
-- schema before it will even look a table up in it. The default,
-- unrelocated case (public) happens to work without this, since
-- public grants USAGE to PUBLIC by default, but a schema named on an
-- explicit SCHEMA clause has no such default and would otherwise
-- leave pg_database_owner with a grant it can never actually use.
--
-- Also granted to PUBLIC: pg_database_owner's own grant carries no
-- GRANT OPTION, so there is no way to pass it on to another role
-- afterward, and the attempt is a silent no-op, not an error.
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
