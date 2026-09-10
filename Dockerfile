##############################
# base image for all flavors #
##############################

# What each chained stage builds FROM. Declared here because an ARG used in a
# FROM must precede the first FROM. The default names the stage above it, giving
# one build graph; a registry reference instead starts that stage from an
# already-published image, which is what a per-flavor CI wave needs.
ARG POSTGRES_IMAGE=postgres
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

#############################
# PostgreSQL-only base image #
#############################
#
# Spock-independent: built once per major, shared across spock lines.

FROM base AS postgres

ARG POSTGRES_PACKAGE_LIST_FILE
ARG TARGETARCH
ARG POSTGRES_MAJOR_VERSION

COPY packagelists/${TARGETARCH}/${POSTGRES_PACKAGE_LIST_FILE} /usr/share/pgedge/packages.txt

RUN <<EOF
#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

xargs dnf install -y < /usr/share/pgedge/packages.txt
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
# minimal-flavored image  #
###########################

FROM ${POSTGRES_IMAGE} AS minimal

ARG PACKAGE_LIST_FILE
ARG TARGETARCH
ARG POSTGRES_MAJOR_VERSION

# The inherited stage ends as USER postgres.
USER root

COPY packagelists/${TARGETARCH}/${PACKAGE_LIST_FILE} /usr/share/pgedge/packages.txt

RUN <<EOF
#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# A delta: re-pinning a package the parent's "dnf update -y" has moved past
# would be a downgrade, which dnf refuses.
xargs dnf install -y < /usr/share/pgedge/packages.txt
dnf update -y
dnf clean all

EOF

USER postgres

###########################
# standard-flavored image #
###########################

# Chained rather than a second FROM base: that is what makes the inherited
# layers byte-identical to minimal's -- as parallel stages they shared 1 of 6.
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

# A delta: re-pinning a package the parent's "dnf update -y" has moved past
# would be a downgrade, which dnf refuses.
xargs dnf install -y < /usr/share/pgedge/packages.txt
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
# FROM standard because ColdFront's vector-tiering path needs pgvector.

FROM ${STANDARD_IMAGE} AS coldfront

# Its own ARG: PACKAGE_LIST_FILE is consumed by an inherited stage, which would
# then COPY this list in place of its own.
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

# Dependencies are dnf's to resolve. No second "dnf update -y": that would move
# ColdFront past its pinned NVR.
xargs dnf install -y --setopt=install_weak_deps=False < /usr/share/pgedge/coldfront-packages.txt
dnf clean all

CFEOF

# Read by coldfront-entrypoint.sh. autoinstall stays off there: the RPM ships
# all four extensions, and with allow_unsigned on it would fetch unsigned code.
# Writable home for the config the entrypoint renders; the packaged one is
# 0600 coldfront:coldfront and this image runs as postgres. Declared, not created
# at runtime, so it is discoverable and mountable under --read-only.
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
