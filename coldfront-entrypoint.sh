#!/usr/bin/env bash
#
# ColdFront wrapper around pgEdge's docker-entrypoint.sh. It appends ColdFront's
# settings as -c arguments and execs that entrypoint, so POSTGRES_* variables,
# _FILE secrets and /docker-entrypoint-initdb.d/ keep working. -c rather than
# writing postgresql.conf: appending on every start would grow the file, and
# args apply to an already-initialised PGDATA. Kubernetes never reaches here --
# CNPG runs its own instance manager and configures from the Cluster spec.

set -o errexit
set -o nounset

# pg_duckdb refuses a non-UTF8 database and pgEdge's initdb defaults to
# SQL_ASCII. Prepended, so an operator's own value still wins.
export POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C ${POSTGRES_INITDB_ARGS:-}"

# --- config for the Go tools -------------------------------------------------
# They read YAML, not GUCs. The packaged /etc/pgedge/coldfront/config.yaml is an
# example owned by the bare-metal service account and unreadable here, so
# environment values are rendered instead.

# section|key|variable, in emission order. An empty variable is skipped: s3 or
# azure configures that backend, and neither selects vended credentials. The
# config format allows exactly one, so setting both is passed through for the
# tools to reject by name -- dropping one here would silently write to the
# other store.
_CF_FIELDS='postgres|dsn|COLDFRONT_PG_DSN
iceberg|warehouse|COLDFRONT_WAREHOUSE
iceberg|lakekeeper_endpoint|COLDFRONT_LAKEKEEPER
s3|endpoint|COLDFRONT_S3_ENDPOINT
s3|region|COLDFRONT_S3_REGION
s3|access_key|COLDFRONT_S3_ACCESS_KEY
s3|secret_key|COLDFRONT_S3_SECRET_KEY
s3|use_ssl|COLDFRONT_S3_USE_SSL
s3|url_style|COLDFRONT_S3_URL_STYLE
azure|connection_string|COLDFRONT_AZURE_CONNECTION_STRING'

# Writes the fields that are set to $1; succeeds only if something was, so the
# caller can tell "unconfigured" from "configured". Single-quoted YAML: the only
# escape is a doubled quote, so nothing else in a DSN or secret can change the
# meaning.
_cf_render_config() {
    local out=$1 emitted='' section key var value
    : > "$out"
    chmod 0600 "$out"

    while IFS='|' read -r section key var; do
        [ -n "$section" ] || continue
        value=${!var-}
        [ -n "$value" ] || continue
        # Fatal, not skipped: skipping would surface as the tool's "no config
        # file" instead of naming the variable at fault.
        case $value in
            *$'\n'*)
                echo "coldfront: ${var} must not contain a newline" >&2
                exit 1
                ;;
        esac
        if [ "$section" != "$emitted" ]; then
            printf '%s:\n' "$section" >> "$out"
            emitted=$section
        fi
        case $value in
            true | false) printf '  %s: %s\n'   "$key" "$value" ;;
            *)            printf "  %s: '%s'\n" "$key" "${value//\'/\'\'}" ;;
        esac >> "$out"
    done <<< "$_CF_FIELDS"

    [ -s "$out" ]
}

case ${1:-} in
archiver | partitioner | compactor)
    _cf_tool=$1
    shift

    _cf_flagged=0
    for _cf_arg; do
        case $_cf_arg in -config | -config=* | --config | --config=*) _cf_flagged=1 ;; esac
    done

    if [ "$_cf_flagged" = 0 ]; then
        # COLDFRONT_CONFIG names a file to READ, never to write, and outranks
        # rendering: COLDFRONT_WAREHOUSE and COLDFRONT_LAKEKEEPER are read by
        # the postgres path too, so they are set even when a file is supplied.
        _cf_rendered=${COLDFRONT_RENDER_CONFIG_TO:-/var/lib/pgedge/coldfront/config.yaml}
        if [ -n "${COLDFRONT_CONFIG:-}" ]; then
            set -- -config "$COLDFRONT_CONFIG" "$@"
        elif _cf_render_config "$_cf_rendered"; then
            echo "coldfront: rendered ${_cf_rendered} from the environment" >&2
            set -- -config "$_cf_rendered" "$@"
        fi
    fi

    exec "/usr/bin/${_cf_tool}" "$@"
    ;;
esac

# libpq keyword/value quoting for the loopback DSN: single-quote the value and
# backslash-escape backslashes and quotes, so a space or quote in a role or
# database name cannot split the DSN or end a value early.
_cf_dsn_quote() {
    printf "'%s'" "$(printf '%s' "$1" | sed "s/[\\\\']/\\\\&/g")"
}

# docker-entrypoint.sh turns a leading option into "postgres $@", but only after
# this wrapper has run. Normalising first keeps `run <image> -c work_mem=...` on
# the branch below instead of silently starting without the preloads.
case ${1:-} in
-*) set -- postgres "$@" ;;
esac

if [ "${1:-}" = "postgres" ]; then
    shift

    # coldfront.so installs DML-routing hooks and an XactCallback, pg_duckdb
    # installs planner and executor hooks, so both need preloading. autoinstall
    # is off because the RPM ships all four loadable extensions and httpfs is
    # compiled into libduckdb -- with allow_unsigned also on, leaving it enabled
    # would mean fetching unsigned code at runtime.
    args=(
        -c shared_preload_libraries="${COLDFRONT_PRELOAD}"
        -c duckdb.extension_directory="${COLDFRONT_EXTENSION_DIR}"
        -c duckdb.autoinstall_known_extensions=false
        -c duckdb.autoload_known_extensions=true
        -c duckdb.allow_unsigned_extensions=true
    )

    # Unset rather than defaulted when absent: coldfront reads these with
    # current_setting(..., true), so the image still starts as a plain node with
    # the extension present but idle.
    if [ -n "${COLDFRONT_WAREHOUSE:-}" ]; then
        args+=(-c coldfront.warehouse="${COLDFRONT_WAREHOUSE}")
    fi
    if [ -n "${COLDFRONT_LAKEKEEPER:-}" ]; then
        args+=(-c coldfront.lakekeeper_endpoint="${COLDFRONT_LAKEKEEPER}")
    fi

    # Loopback DSN for coldfront.ensure_pg_attached(). Derived from the same
    # variables pgEdge's entrypoint uses for the role and database, following
    # its defaulting (POSTGRES_DB falls back to POSTGRES_USER). Override where
    # the socket lives elsewhere -- CNPG forces /controller/run.
    if [ -z "${COLDFRONT_LOCAL_PG_DSN:-}" ]; then
        _cf_user="${POSTGRES_USER:-postgres}"
        COLDFRONT_LOCAL_PG_DSN="host=$(_cf_dsn_quote "${COLDFRONT_SOCKET_DIR:-/var/run/postgresql}")"
        COLDFRONT_LOCAL_PG_DSN+=" dbname=$(_cf_dsn_quote "${POSTGRES_DB:-${_cf_user}}")"
        COLDFRONT_LOCAL_PG_DSN+=" user=$(_cf_dsn_quote "${_cf_user}")"
        COLDFRONT_LOCAL_PG_DSN+=" application_name=coldfront_pglocal"
    fi
    args+=(-c coldfront.local_pg_dsn="${COLDFRONT_LOCAL_PG_DSN}")

    # pg_duckdb gates DuckDB on membership of this role; unset keeps its stock
    # superuser-only default.
    if [ -n "${COLDFRONT_DUCKDB_ROLE:-}" ]; then
        args+=(-c duckdb.postgres_role="${COLDFRONT_DUCKDB_ROLE}")
    fi

    # Operator arguments last, so an explicit -c wins.
    set -- postgres "${args[@]}" "$@"
fi

exec /usr/local/bin/docker-entrypoint.sh "$@"
