#!/usr/bin/env bash
#
# ColdFront wrapper around pgEdge's docker-entrypoint.sh.
#
# It does NOT replace that entrypoint: it appends ColdFront's postmaster-start
# GUCs as -c arguments and then execs it, so POSTGRES_* variables, _FILE secrets
# and /docker-entrypoint-initdb.d/ all keep working.
#
# -c arguments rather than writing postgresql.conf because:
#   * appending on every start would grow the file without bound;
#   * they apply to an already-initialised PGDATA, which an initdb.d script cannot;
#   * pgEdge's entrypoint forwards "$@" into docker_temp_server_start, so the
#     preload is live during initdb.d and an init script can CREATE EXTENSION.
#
# Kubernetes never reaches this file: CNPG runs its own instance manager and
# takes these settings from the Cluster spec instead.

set -o errexit
set -o nounset

# pg_duckdb refuses to install in a non-UTF8 database, and pgEdge's initdb
# defaults to SQL_ASCII. Prepend the encoding ColdFront needs while preserving
# anything the operator passed, so an explicit override still wins.
export POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C ${POSTGRES_INITDB_ARGS:-}"

if [ "${1:-}" = "postgres" ]; then
    shift
    set -- postgres \
        -c shared_preload_libraries="${COLDFRONT_PRELOAD}" \
        -c duckdb.extension_directory="${COLDFRONT_EXTENSION_DIR}" \
        -c duckdb.autoinstall_known_extensions=false \
        -c duckdb.autoload_known_extensions=true \
        -c duckdb.allow_unsigned_extensions=true \
        "$@"
fi

exec /usr/local/bin/docker-entrypoint.sh "$@"
