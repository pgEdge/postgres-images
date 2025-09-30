# How to run this example

```sh
docker compose up -d
```

# How to interact with this example

This configuration creates a database called acctg that's replicated between two pgEdge nodes.

## Connect to `acctg` with Docker

To open a `psql` session on the first node, run:
```sh
docker compose exec postgres-n1 psql -U pgedge acctg
```

Likewise, to open a `psql` session on the second node, run:
```sh
docker compose exec postgres-n2 psql -U pgedge acctg
```

## Try out replication

1. Create a table on the first node:
```sh
docker compose exec postgres-n1 psql -U pgedge acctg -c "create table example (id int primary key, data text);"
```
2. Insert a row into our new table on the second node:
```sh
docker compose exec postgres-n2 psql -U pgedge acctg -c "insert into example (id, data) values (1, 'Hello, pgEdge!');"
```
3. See that the new row has replicated back to the first node:
```sh
docker compose exec postgres-n1 psql -U pgedge acctg -c "select * from example;"
```

## Load the Northwind example dataset

The Northwind example dataset is a PostgreSQL database dump that you can use to try replication with a more realistic database.  To load the Northwind dataset into your pgEdge database, run:

```sh
curl https://downloads.pgedge.com/platform/examples/northwind/northwind.sql | docker compose exec -T postgres-n1 psql -U pgedge acctg
```

Now, try querying one of the new tables from the other node:

```sh
docker compose exec postgres-n2 psql -U pgedge acctg -c "select * from northwind.shippers"
```

## Connect to `acctg` from another client

If you have `psql`, pgAdmin, or another client installed on your host machine, you can use these connection strings to connect to each node:

- First node: `host=localhost port=6432 user=pgedge password=pgedge dbname=acctg`
- Second node: `host=localhost port=6433 user=pgedge password=pgedge dbname=acctg`

For example, using `psql`:

```sh
psql 'host=localhost port=6432 user=pgedge password=pgedge dbname=acctg'
```

# How to modify this example

You can adjust settings in the docker-compose.yaml under each service’s environment:

- POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB — database superuser, password, and database name (default: pgedge / pgedge / acctg)
- PGEDGE_USER, PGEDGE_PASSWORD — application/replication user and password (default: pgedge / pgedge)
- `nodes`: Configures the pgEdge nodes.
- `users`: Configures which users will be created on each pgEdge node.
- NODE_NAME — logical node name for Spock (n1 / n2)
- Published ports are set under ports (6432 for postgres-n1, 6433 for postgres-n2)


Note that this configuration only takes effect when the containers are first created. To recreate the database with a new configuration, stop the running example:

```sh
docker compose down
```

And start it again:

```sh
docker compose up -d
```
