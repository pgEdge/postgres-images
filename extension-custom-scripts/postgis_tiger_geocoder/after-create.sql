-- Read access to the reference tables the extension's own functions
-- query (tiger.*, created at CREATE EXTENSION time). The actual
-- Census dataset a user bulk-loads afterward is a separate step this
-- script cannot reach, it only runs once, at CREATE EXTENSION time.
--
-- Also granted to PUBLIC: pg_database_owner's own grant carries no
-- GRANT OPTION, so there is no way to pass it on to another role
-- afterward, and the attempt is a silent no-op, not an error.
GRANT USAGE ON SCHEMA tiger TO pg_database_owner, PUBLIC;
GRANT SELECT ON ALL TABLES IN SCHEMA tiger TO pg_database_owner, PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tiger
  GRANT SELECT ON TABLES TO pg_database_owner, PUBLIC;
