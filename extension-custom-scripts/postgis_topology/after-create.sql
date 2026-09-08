-- PostGIS's own install script grants PUBLIC only read access to the
-- topology schema (USAGE on the schema, SELECT on its tables), enough
-- to read an existing topology, not to create one. CreateTopology()
-- and AddTopoGeometryColumn(), the extension's actual purpose, both
-- write to tables it creates: topology.topology, topology.layer, and
-- topology.topology_id_seq. Scoped to exactly these three objects
-- rather than a schema-wide grant, since broadening a similar
-- database-wide default was tried elsewhere and rejected: it would
-- also hand write access to other gated extensions' catalogs that are
-- meant to stay admin-only.
--
-- Granted to pg_database_owner rather than a hardcoded role name, so
-- this keeps working if the database is later reassigned to a
-- different owner. See
-- https://www.postgresql.org/docs/current/predefined-roles.html.
GRANT INSERT ON topology.topology TO pg_database_owner;
GRANT USAGE ON SEQUENCE topology.topology_id_seq TO pg_database_owner;
GRANT INSERT ON topology.layer TO pg_database_owner;
