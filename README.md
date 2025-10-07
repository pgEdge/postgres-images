# pgEdge Postgres Images

This repository provides build scripts for generating pgEdge Postgres container images supporting Postgres versions 16 and 17.

Images are built from pgEdge Enterprise Postgres packages using a rockylinux9-ubi base image. 

Images are published on the [pgEdge Github Container Registry](https://github.com/orgs/pgEdge/packages/container/package/pgedge-postgres).

## Image Flavors

There are currently 2 supported image flavors: `minimal` and `standard`. 

Package lists contained under `packagelists` show the exact contents of each image version.

### Minimal Images

Minimal images contain Postgres, and the following extensions:

- Spock
- LOLOR
- Snowflake

### Standard Images

Standard images are based on minimal images, and contain additional extension and tools.

- PGAudit
- PostGIS
- pgVector
- Patroni
- pgBackRest
- psycopg2

## Entry Points

The default entry point for this image is based on the official Postgres image entrypoint. Documentation on supported entrypoint behavior is located in the [docker-library/docs repo](https://github.com/docker-library/docs/blob/master/postgres/README.md). Running the container as root is not currently supported.

In addition to the default entry point, Patroni (`/usr/local/bin/patroni`) can also be used as an entrypoint in the standard image.

## Examples

### docker run

To run a single instance you can use this command:

```
docker run --name pgedge-postgres \
  -e POSTGRES_PASSWORD=mypassword \
  -e POSTGRES_USER=admin \
  -e POSTGRES_DB=example_db \
  -p 6432:5432 \
  -d ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
```

You can then log in using `psql` with the following command:

```
docker exec -it pgedge-postgres psql -U admin example_db
```

### docker compose

This repository includes two Compose examples you can try out:

- Enterprise Example

Runs a single Postgres instance using the standard image and initializes extensions.

`cd examples/compose/enterprise`
`docker compose up`

This example utilizes the standard image and handles initializing and creating extensions.

- Distributed Example

Runs two pgEdge Postgres nodes (n1 / n2) with Spock logical replication preconfigured.
This example demonstrates bi-directional replication and includes instructions for testing with the Northwind dataset.
Once Postgres initialization completes, you can connect to the database in a separate shell:

`cd examples/compose/distributed`
`docker compose up -d`



## Data Volumes

This image is compatible with Docker volumes and bind mounts. The configuration
for both is similar. Because Postgres requires the data directory to be owned
by the user running the database, the `PGDATA` directory should be specified as
a subdirectory of the volume mount.

By default, this image uses the following approach for volume configuration:

- `/var/lib/pgsql` is the recommended volume mount point
- `/var/lib/pgsql/<pg_major_version>/data` is the default Postgres data directory (`PGDATA`)

An example Docker compose spec that shows this looks like this:

```yaml
pgedge-postgres:
    image: ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
    restart: always
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-admin}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-password}
      POSTGRES_DB: ${POSTGRES_DB:-example_db}
    volumes:
      - data:/var/lib/pgsql

volumes:
  data:
```

## Image Tags

- Every image will have an immutable tag, `<postgres major.minor>-spock<major.minor.patch>-<flavor>-<epoch>`, e.g. `17.6-spock5.0.0-standard-1`
- Mutable tags also exist for:
  - The latest image for a given Postgres major.minor + spock major version, `pg<postgres major.minor>-spock<major>-<flavor>` , e.g. `17.6-spock5-standard`
  - The latest image for a given Postgres major + spock major version, `pg<postgres major>-spock<major>-<flavor>`, e.g. `17-spock5-standard`
