# pgEdge Enterprise Postgres — Quick Start

This example spins up a single pgEdge Enterprise Postgres container with additional enterprise extensions enabled (pgAudit, PostGIS, Snowflake, Spock, etc.). The container is configured with logical replication support and initializes all extensions automatically at startup.

## Prerequisites

- Docker & Docker Compose installed

- Port 6432 available on your host machine

- Internet access to pull container images

### Using this Docker File

The docker-compose.yaml file in this repository creates a Postgres database named example_db with enterprise extensions pre-loaded. Before running this file, ensure that you have Docker installed and running.

To deploy this example, run:

```sh
docker compose up -d
```

This will build and start the pgEdge Enterprise Postgres service.

Connecting to example_db with psql

docker compose exec pgedge-postgres psql -U admin example_db

### Enterprise Extensions

This enterprise image automatically enables and installs the following extensions:

- pg_stat_statements

- pgAudit

- Snowflake

- Spock

- PostGIS

These are configured in two phases:

The init-extensions and configure-spock scripts update postgresql.conf with preload libraries and Spock settings.

The create-extensions script runs CREATE EXTENSION commands to load them into your database.

You can confirm extensions are installed by running:
```sh
\dx
```
inside your psql session.

### Restarting PostgreSQL During Init

To apply configuration changes, the initialization sequence includes a controlled restart of PostgreSQL:
```sh
pg_ctl -D $PGDATA -m fast restart
```

This happens automatically during first startup. You don’t need to run this manually unless you change configuration.

### Loading Sample Data

You can load the Northwind sample dataset into your enterprise Postgres instance by running:
```sh
curl https://downloads.pgedge.com/platform/examples/northwind/northwind.sql | docker compose exec -T pgedge-postgres psql -U admin example_db
```

Once loaded, verify with a query such as:
```sh
docker compose exec pgedge-postgres psql -U admin example_db -c "select * from northwind.shippers;"
```
Connecting from Outside the Container

If you have psql, pgAdmin, or another Postgres client installed on your host machine, use this connection string:

host=localhost port=6432 user=admin password=password dbname=example_db


For example, with psql:

psql 'host=localhost port=6432 user=admin password=password dbname=example_db'

Modifying this Example

You can adjust environment variables in docker-compose.yaml to change default credentials and database name:

environment:
-  POSTGRES_USER: admin
-  POSTGRES_PASSWORD: password
-  POSTGRES_DB: example_db

POSTGRES_USER: database superuser (default: admin).

POSTGRES_PASSWORD: password for the superuser (default: password).

POSTGRES_DB: database name (default: example_db).

The published port can also be changed:

ports:
  - target: 5432
    published: 6432


Change 6432 if you need to run multiple Postgres services on the same host.

Recreating with a Fresh Configuration

This setup only applies during initial container creation. If you want to restart with a fresh database:
```sh
docker compose down -v
docker compose up -d
```

The -v flag ensures volumes are removed so configuration and extension initialization re-runs.