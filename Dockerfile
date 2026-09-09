##############################
# base image for all flavors #
##############################

# Global ARG: an ARG used in a FROM instruction has to be declared before the
# first FROM, outside any stage. See the coldfront stage for what it selects.
ARG MINIMAL_IMAGE=minimal
ARG STANDARD_IMAGE=standard

FROM rockylinux/rockylinux:9-ubi AS base

ARG PACKAGE_RELEASE_CHANNEL=""
ARG POSTGRES_USER_ID="26"
COPY docker-entrypoint.sh docker-ensure-initdb.sh /usr/local/bin/

RUN <<EOF
#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

useradd -u ${POSTGRES_USER_ID} -m postgres -s /bin/bash

cat >> /etc/dnf/dnf.conf <<'CONF'
retries=10
timeout=30
minrate=100k
fastestmirror=1
max_parallel_downloads=10
CONF

dnf install -y epel-release dnf
dnf config-manager --set-enabled crb
dnf update -y --allowerasing
dnf install -y https://dnf.pgedge.com/reporpm/pgedge-release-latest.noarch.rpm
if [[ -n "${PACKAGE_RELEASE_CHANNEL}" ]]; then
    sed -i "s|release|${PACKAGE_RELEASE_CHANNEL}|g" /etc/yum.repos.d/pgedge.repo
fi

mkdir /docker-entrypoint-initdb.d

EOF

##########################
# minimal-flavored image #
##########################

FROM base AS minimal

ARG PACKAGE_LIST_FILE
ARG TARGETARCH
ARG POSTGRES_MAJOR_VERSION

COPY packagelists/${TARGETARCH}/${PACKAGE_LIST_FILE} /usr/share/pgedge/packages.txt

RUN <<EOF
#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

grep -vE '^[[:space:]]*(#|$)' /usr/share/pgedge/packages.txt | xargs dnf install -y
# Patch any OS packages (including transitive dependencies pulled in above)
# to the latest available errata so the image ships with security fixes.
dnf update -y
dnf clean all

EOF

ENV PGDATA=/var/lib/pgsql/${POSTGRES_MAJOR_VERSION}/data
RUN install --verbose --directory --owner postgres --group postgres --mode 1777 "$PGDATA"

USER postgres

ENV PG_MAJOR=${POSTGRES_MAJOR_VERSION}
ENV PATH=$PATH:/usr/pgsql-${POSTGRES_MAJOR_VERSION}/bin

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

# We set the default STOPSIGNAL to SIGINT, which corresponds to what PostgreSQL
# calls "Fast Shutdown mode" wherein new connections are disallowed and any
# in-progress transactions are aborted, allowing PostgreSQL to stop cleanly and
# flush tables to disk.
#
# See https://www.postgresql.org/docs/current/server-shutdown.html for more details
# about available PostgreSQL server shutdown signals.
#
# See also https://www.postgresql.org/docs/current/server-start.html for further
# justification of this as the default value, namely that the example (and
# shipped) systemd service files use the "Fast Shutdown mode" for service
# termination.
#
STOPSIGNAL SIGINT
#
# An additional setting that is recommended for all users regardless of this
# value is the runtime "--stop-timeout" (or your orchestrator/runtime's
# equivalent) for controlling how long to wait between sending the defined
# STOPSIGNAL and sending SIGKILL.
#
# The default in most runtimes (such as Docker) is 10 seconds, and the
# documentation at https://www.postgresql.org/docs/current/server-start.html notes
# that even 90 seconds may not be long enough in many instances.

EXPOSE 5432
CMD ["postgres"]

###########################
# standard-flavored image #
###########################

# MINIMAL_IMAGE works exactly like STANDARD_IMAGE on the coldfront stage: the
# default resolves to the stage above for a single-graph build, and a registry
# reference makes this stage start from an already-published minimal, which is
# what a per-flavor CI wave needs. Chaining rather than a second FROM base is
# what makes standard's inherited layers byte-identical to minimal's -- as
# parallel stages they shared only 1 of 6.
FROM ${MINIMAL_IMAGE} AS standard

ARG STANDARD_PACKAGE_LIST_FILE
ARG TARGETARCH
ARG POSTGRES_MAJOR_VERSION

# The inherited stage ends as USER postgres.
USER root

COPY packagelists/${TARGETARCH}/${STANDARD_PACKAGE_LIST_FILE} /usr/share/pgedge/packages.txt

RUN <<EOF
#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# A delta over minimal, not a full manifest: re-pinning a package that minimal's
# "dnf update -y" has already moved past its NVR is a downgrade request, which
# dnf refuses. Comments and blank lines are stripped so the list can document
# its own chain.
grep -vE '^[[:space:]]*(#|$)' /usr/share/pgedge/packages.txt | xargs dnf install -y
# Patch any OS packages (including transitive dependencies pulled in above)
# to the latest available errata so the image ships with security fixes.
dnf update -y
# Patroni's HTTP client dependencies only ship security fixes for Python >= 3.10
# (urllib3 >= 2.7.0, requests >= 2.33.0), while the system python3 is 3.9. Install
# patroni under python3.12 so it can pull the patched urllib3/requests, resolving
# CVE-2026-44431 and CVE-2026-44432 (urllib3) and CVE-2026-25645 (requests). The
# floors are pinned explicitly (patroni does not constrain them) so the build
# fails loudly if a fixed version is ever unavailable. The psycopg3 extra bundles
# libpq, so patroni no longer relies on the Python 3.9-only system psycopg2.
dnf install -y python3.12 python3.12-pip
python3.12 -m pip install 'patroni[etcd,jsonlogger,psycopg3]==4.1.3' 'urllib3>=2.7.0' 'requests>=2.33.0'
dnf remove -y python3.12-pip
dnf clean all

EOF

ENV PGDATA=/var/lib/pgsql/${POSTGRES_MAJOR_VERSION}/data
RUN install --verbose --directory --owner postgres --group postgres --mode 1777 "$PGDATA"

USER postgres

ENV PG_MAJOR=${POSTGRES_MAJOR_VERSION}
ENV PATH=$PATH:/usr/pgsql-${POSTGRES_MAJOR_VERSION}/bin

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

# We set the default STOPSIGNAL to SIGINT, which corresponds to what PostgreSQL
# calls "Fast Shutdown mode" wherein new connections are disallowed and any
# in-progress transactions are aborted, allowing PostgreSQL to stop cleanly and
# flush tables to disk.
#
# See https://www.postgresql.org/docs/current/server-shutdown.html for more details
# about available PostgreSQL server shutdown signals.
#
# See also https://www.postgresql.org/docs/current/server-start.html for further
# justification of this as the default value, namely that the example (and
# shipped) systemd service files use the "Fast Shutdown mode" for service
# termination.
#
STOPSIGNAL SIGINT
#
# An additional setting that is recommended for all users regardless of this
# value is the runtime "--stop-timeout" (or your orchestrator/runtime's
# equivalent) for controlling how long to wait between sending the defined
# STOPSIGNAL and sending SIGKILL.
#
# The default in most runtimes (such as Docker) is 10 seconds, and the
# documentation at https://www.postgresql.org/docs/current/server-start.html notes
# that even 90 seconds may not be long enough in many instances.

EXPOSE 5432
CMD ["postgres"]

############################
# coldfront-flavored image #
############################
#
# Chained FROM standard rather than FROM base: ColdFront's vector-tiering path
# needs pgvector, which only standard ships. Chaining also makes the coldfront
# layers a genuine delta over standard rather than a parallel full install, so
# the inherited layers keep byte-identical digests.

# STANDARD_IMAGE selects what this flavor is chained from, and supports both
# build models:
#   * default "standard" resolves to the stage above, so a local or single-call
#     build produces one graph in which the standard stage is built exactly once
#     and both images provably inherit the same layers;
#   * a registry reference (ideally digest-pinned) makes this stage start from an
#     already-published standard, which is what a per-flavor CI wave needs -- the
#     coldfront job runs on a different runner than the standard job, so
#     rebuilding the stage there would re-run its unpinned "dnf update -y" and
#     yield different layers.
FROM ${STANDARD_IMAGE} AS coldfront

# A separate ARG is required. This stage cannot reuse PACKAGE_LIST_FILE, because
# that ARG is consumed by the inherited standard stage -- passing the coldfront
# list through it would make standard COPY the coldfront list in place of its own.
ARG COLDFRONT_PACKAGE_LIST_FILE
ARG TARGETARCH
ARG POSTGRES_MAJOR_VERSION

USER root

COPY packagelists/${TARGETARCH}/${COLDFRONT_PACKAGE_LIST_FILE} /usr/share/pgedge/coldfront-packages.txt

RUN <<CFEOF
#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# Only what this stage installs is pinned. pg-duckdb and the DuckDB extensions
# are dependencies of pgedge-coldfront_<major> and are dnf's to resolve.
# Deliberately no second "dnf update -y": the inherited standard layers already
# ran one, and repeating it here would move ColdFront past its pinned NVR.
grep -vE '^[[:space:]]*(#|$)' /usr/share/pgedge/coldfront-packages.txt \
    | xargs dnf install -y --setopt=install_weak_deps=False
dnf clean all

CFEOF

# ColdFront needs pg_duckdb and coldfront preloaded at postmaster start, and
# DuckDB's extensions come from the read-only RPM path. autoinstall is off: the
# RPM ships all four loadable extensions and httpfs is compiled into libduckdb,
# so nothing needs fetching -- and with allow_unsigned on, autoinstall would mean
# loading unsigned code from the network at runtime.
# A writable home for configuration the entrypoint renders from the environment.
# The packaged /etc/pgedge/coldfront/config.yaml cannot serve: it is
# 0600 coldfront:coldfront for the bare-metal service account, while this image
# runs as postgres. Declared here rather than created at runtime so the path is
# discoverable, and so it can be mounted -- which is what a --read-only root
# filesystem needs.
RUN install --verbose --directory --owner postgres --group postgres --mode 0700 \
        /var/lib/pgedge/coldfront

ENV COLDFRONT_PRELOAD="pg_duckdb,coldfront"
ENV COLDFRONT_EXTENSION_DIR="/usr/lib/pgedge/coldfront/duckdb-extensions"

COPY coldfront-entrypoint.sh /usr/local/bin/

USER postgres

ENTRYPOINT ["/usr/local/bin/coldfront-entrypoint.sh"]
STOPSIGNAL SIGINT
EXPOSE 5432
CMD ["postgres"]
