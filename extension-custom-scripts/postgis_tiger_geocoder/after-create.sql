-- The database's own owner gets read access to the reference tables the
-- extension's own functions query (tiger.*, created at CREATE EXTENSION
-- time).
--
-- The actual Census dataset a user bulk-loads afterward is a separate
-- step this script cannot reach (it only runs once, at CREATE
-- EXTENSION time): loading it is ordinary CREATE TABLE/COPY, nothing
-- here requires supautils' gate or any elevated role, so running that
-- load connected as the database's own owner lands it owned by that
-- role directly, with no extra grant needed.
--
-- Granted to pg_database_owner rather than a hardcoded role name, so
-- this keeps working if the database is later reassigned to a
-- different owner. See
-- https://www.postgresql.org/docs/current/predefined-roles.html.
GRANT USAGE ON SCHEMA tiger TO pg_database_owner;
GRANT SELECT ON ALL TABLES IN SCHEMA tiger TO pg_database_owner;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tiger
  GRANT SELECT ON TABLES TO pg_database_owner;
