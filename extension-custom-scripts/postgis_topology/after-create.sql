-- Every role can read existing topologies; PostGIS grants that
-- itself. The database's owner can also create, rename, and drop
-- topologies, and add and drop topology columns, all of which write
-- topology.topology and topology.layer.
--
-- The two tables stay owned by the installing superuser, so the
-- database's owner cannot attach triggers to them or change their
-- structure. RenameTopoGeometryColumn() disables and re-enables
-- triggers on topology.layer, which only the table's owner can do,
-- so it is not available to the database's owner. Granting the
-- TRIGGER privilege does not help: a role with it and full DML on
-- both tables still gets "must be owner of table layer".
GRANT SELECT, INSERT, UPDATE, DELETE ON topology.topology TO pg_database_owner;
GRANT SELECT, INSERT, UPDATE, DELETE ON topology.layer TO pg_database_owner;
GRANT USAGE, SELECT ON SEQUENCE topology.topology_id_seq TO pg_database_owner;
