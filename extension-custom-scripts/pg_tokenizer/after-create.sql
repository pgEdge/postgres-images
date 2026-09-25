-- Every role can read tokenizer_catalog and use an existing
-- tokenizer or text analyzer through tokenize() and
-- apply_text_analyzer(). Only the database's owner can create, change,
-- or drop tokenizers, text analyzers, models, stopword lists, and
-- synonym lists.
--
-- The configuration functions run as the caller and write the
-- catalog tables with ordinary INSERT, UPDATE, and DELETE, so the
-- owner needs write access to the tables as well as EXECUTE on the
-- functions. Default privileges extend the same access to tables and
-- sequences added to the schema later.
--
-- No role but the installing superuser can create objects in
-- tokenizer_catalog. Custom models are therefore not available to
-- the database's owner, since create_custom_model() and
-- create_custom_model_tokenizer_and_trigger() create a vocabulary
-- table in this schema. Tokenizers built on the preloaded models
-- work.
GRANT USAGE ON SCHEMA tokenizer_catalog TO pg_database_owner, PUBLIC;
GRANT SELECT ON ALL TABLES IN SCHEMA tokenizer_catalog TO pg_database_owner, PUBLIC;
GRANT INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA tokenizer_catalog TO pg_database_owner;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA tokenizer_catalog TO pg_database_owner;

ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tokenizer_catalog
  GRANT SELECT ON TABLES TO pg_database_owner, PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tokenizer_catalog
  GRANT INSERT, UPDATE, DELETE ON TABLES TO pg_database_owner;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA tokenizer_catalog
  GRANT USAGE, SELECT ON SEQUENCES TO pg_database_owner;

-- EXECUTE is limited separately from table access because some model
-- functions, create_huggingface_model() and create_lindera_model()
-- among them, parse their configuration and load a model before they
-- write anything. A role without EXECUTE is refused before that work
-- starts; a table permission alone would refuse it only afterward.
-- PUBLIC keeps the three read-only functions.
--
-- This covers only the functions that exist when the script runs. A
-- function an extension upgrade adds to tokenizer_catalog gets
-- Postgres' default of EXECUTE for PUBLIC, and default privileges
-- cannot prevent that: one scoped IN SCHEMA can only add to the
-- default, never revoke from it, and one without IN SCHEMA would
-- revoke PUBLIC EXECUTE from every function the installing superuser
-- creates anywhere in the database. After an upgrade, run this script
-- again.
REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA tokenizer_catalog FROM PUBLIC;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA tokenizer_catalog TO pg_database_owner;
GRANT EXECUTE ON FUNCTION tokenizer_catalog.tokenize(text, text) TO PUBLIC;
GRANT EXECUTE ON FUNCTION tokenizer_catalog.apply_text_analyzer(text, text) TO PUBLIC;
GRANT EXECUTE ON FUNCTION tokenizer_catalog.list_preload_models() TO PUBLIC;
