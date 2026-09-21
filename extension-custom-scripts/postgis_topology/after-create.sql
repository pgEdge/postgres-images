-- PostGIS's own install script grants PUBLIC only read access to the
-- topology schema (USAGE on the schema, SELECT on its tables), enough
-- to read an existing topology, not to manage one. The extension's
-- actual purpose needs a lot more than INSERT on topology.topology and
-- topology.layer: DropTopology()/DropTopoGeometryColumn() DELETE from
-- both, RenameTopology()/RenameTopoGeometryColumn() UPDATE them, and
-- RenameTopoGeometryColumn() additionally runs ALTER TABLE ...
-- DISABLE/ENABLE TRIGGER on topology.layer, which Postgres never
-- grants, only an owner (or superuser) can do it. Unlike pg_cron,
-- whose write functions bypass ACL checks through internal C code,
-- postgis_topology's functions run as the caller through ordinary
-- ACL-checked DML, so a grant-only approach can't cover the trigger
-- toggle: confirmed directly, a role with full DML and even the
-- TRIGGER privilege on both tables still gets "must be owner of table
-- layer" from RenameTopoGeometryColumn(). Reassigning ownership of
-- exactly these two tables is the only way to cover all of that.
-- Scoped to exactly these two tables rather than the whole schema, so
-- it doesn't also hand write access to other gated extensions'
-- catalogs that are meant to stay admin-only.
--
-- Reassigned to pg_database_owner rather than a hardcoded role name,
-- so this keeps working if the database is later reassigned to a
-- different owner. See
-- https://www.postgresql.org/docs/current/predefined-roles.html.
ALTER TABLE topology.topology OWNER TO pg_database_owner;
ALTER TABLE topology.layer OWNER TO pg_database_owner;
GRANT USAGE ON SEQUENCE topology.topology_id_seq TO pg_database_owner;
