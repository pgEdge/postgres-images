-- Every role can read the tables in the tiger schema, which is enough
-- for normalize_address(), pprint_addy(), and geocode(). No role but
-- the installing superuser can modify them. The default privileges
-- below cover tables that same superuser adds to tiger later, not
-- tables another role creates there.
--
-- Loading the Census TIGER dataset still requires a superuser: the
-- loader creates tables in tiger_data that inherit from the ones in
-- tiger, and inheriting from a table requires owning it. Nothing here
-- grants access to tiger_data. Queries through the parent tables in
-- tiger still see the loaded rows, because Postgres checks
-- permissions only on the parent of an inherited query.
GRANT USAGE ON SCHEMA tiger TO pg_database_owner, PUBLIC;
GRANT SELECT ON ALL TABLES IN SCHEMA tiger TO pg_database_owner, PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tiger
  GRANT SELECT ON TABLES TO pg_database_owner, PUBLIC;
