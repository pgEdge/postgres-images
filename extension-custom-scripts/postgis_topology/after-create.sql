-- PostGIS's own install script grants PUBLIC only read access to the
-- topology schema (USAGE on the schema, SELECT on its tables), enough
-- to read an existing topology, not to create one. CreateTopology()
-- and AddTopoGeometryColumn(), the extension's actual purpose, both
-- write to tables it creates: topology.topology, topology.layer, and
-- topology.topology_id_seq. Scoped to exactly these three objects
-- rather than a schema-wide grant: broadening the cluster-wide
-- default-privilege pattern used for other extensions was tried and
-- rejected, since it would also hand app write access to
-- pg_tokenizer's catalogs, a boundary kept deliberately admin-only.
--
-- Runs only when a role named "app" exists (see pg_cron's
-- after-create.sql for why): a no-op otherwise.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
    GRANT INSERT ON topology.topology TO app;
    GRANT USAGE ON SEQUENCE topology.topology_id_seq TO app;
    GRANT INSERT ON topology.layer TO app;
  END IF;
END;
$$;
