# pgEdge Enterprise Postgres - Docker Compose Example

This example spins up a single pgEdge Enterprise Postgres container with enterprise extensions enabled.

## Prerequisites

- Install Docker and Docker Compose on your host system.
- Ensure that port 6432 is available on your host system.
- Ensure that the host system has internet access to pull container images.

### Using this Docker File

The docker-compose.yaml file in this repository creates a Postgres database named example_db with enterprise extensions pre-loaded. Before running this file, ensure that you have Docker installed and running.

To deploy this example, run:

```sh
docker compose up -d
```

This will build and start the pgEdge Enterprise Postgres service.

#### Connecting to example_db with psql

```shell
docker compose exec pgedge-postgres psql -U admin example_db
```

### Loading Sample Data

You can load the Northwind sample dataset into your Postgres database by running:

```sh
curl https://downloads.pgedge.com/platform/examples/northwind/northwind.sql | docker compose exec -T pgedge-postgres psql -U admin example_db
```

Once loaded, verify with a query such as:

```sh
docker compose exec pgedge-postgres psql -U admin example_db -c "select * from northwind.shippers;"
```

## Connecting from Outside the Container

If you have psql, pgAdmin, or another Postgres client installed on your host machine, use this connection string:

`host=localhost port=6432 user=admin password=password dbname=example_db`

For example, with psql:

`psql 'host=localhost port=6432 user=admin password=password dbname=example_db'`

## Modifying this Example

You can adjust environment variables in docker-compose.yaml to change default credentials and database name:

environment:

- POSTGRES_USER: admin
  - database superuser (default: admin).
- POSTGRES_PASSWORD: password
  - password for the superuser (default: password).
- POSTGRES_DB: example_db
  - database name (default: example_db).

The published port can also be changed:

```yaml
    ports:
      - target: 5432
        published: 6432
```

Change 6432 if you need to run multiple Postgres services on the same host.

## Recreating the container

This setup only applies during initial container creation. If you want to restart with a fresh database:

```sh
docker compose down -v
docker compose up -d
```

The -v flag ensures volumes are removed so configuration and extension initialization re-runs.
