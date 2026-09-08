-- PostGIS's own install script grants PUBLIC only read access to the
-- topology schema (USAGE on the schema, SELECT on its tables), enough
-- to read an existing topology, not to manage one. The extension's
-- actual purpose needs a lot more than INSERT on topology.topology and
-- topology.layer: DropTopology()/DropTopoGeometryColumn() DELETE from
-- both, RenameTopology()/RenameTopoGeometryColumn() UPDATE them, and
-- the rename path also runs ALTER TABLE ... DISABLE/ENABLE TRIGGER on
-- topology.layer, which Postgres never grants, only an owner (or
-- superuser) can do it. Reassigning ownership of exactly these two
-- tables covers all of that in one step, the same approach used for
-- pg_cron's job tables. Scoped to exactly these two tables rather than
-- the whole schema, since broadening a similar database-wide default
-- was tried elsewhere and rejected: it would also hand write access to
-- other gated extensions' catalogs that are meant to stay admin-only.
--
-- Reassigned to pg_database_owner rather than a hardcoded role name,
-- so this keeps working if the database is later reassigned to a
-- different owner. See
-- https://www.postgresql.org/docs/current/predefined-roles.html.
ALTER TABLE topology.topology OWNER TO pg_database_owner;
ALTER TABLE topology.layer OWNER TO pg_database_owner;
GRANT USAGE ON SEQUENCE topology.topology_id_seq TO pg_database_owner;
