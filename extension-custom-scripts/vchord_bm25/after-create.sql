-- USAGE on bm25_catalog is needed to declare a column of its
-- bm25vector type and call its functions (search_bm25query,
-- to_bm25query); Postgres already grants EXECUTE on new functions to
-- PUBLIC by default, so no separate function grant is needed. The
-- schema holds only the type and its support functions, no tables.
--
-- Also granted to PUBLIC: pg_database_owner's own grant carries no
-- GRANT OPTION, so there is no way to pass it on to another role
-- afterward, and the attempt is a silent no-op, not an error.
GRANT USAGE ON SCHEMA bm25_catalog TO pg_database_owner, PUBLIC;
