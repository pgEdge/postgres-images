-- Every role can read the tables in the tiger schema, including
-- tables added to it later, which is enough for normalize_address(),
-- pprint_addy(), and geocode() against whatever data is loaded. No
-- role but the installing superuser can modify them.
--
-- Loading the Census TIGER dataset still requires a superuser: the
-- loader creates tables that inherit from the ones in tiger, and
-- inheriting from a table requires owning it.
GRANT USAGE ON SCHEMA tiger TO pg_database_owner, PUBLIC;
GRANT SELECT ON ALL TABLES IN SCHEMA tiger TO pg_database_owner, PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tiger
  GRANT SELECT ON TABLES TO pg_database_owner, PUBLIC;
