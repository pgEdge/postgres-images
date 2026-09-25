-- Every role can declare bm25vector columns, build bm25 indexes, and
-- query them with to_bm25query() and search_bm25query(). The schema
-- holds only the type and its support functions, and Postgres
-- already grants EXECUTE on those to PUBLIC, so schema USAGE is the
-- only grant needed.
GRANT USAGE ON SCHEMA bm25_catalog TO pg_database_owner, PUBLIC;
