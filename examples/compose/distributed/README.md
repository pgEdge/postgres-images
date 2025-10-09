# pgEdge Distributed Postgres  - Docker Compose Example

This example spins up two pgEdge containers (postgres-n1, postgres-n2) and configures logical replication between them using Spock. A database called example_db is created on both nodes and changes replicate bi-directionally.

## Prerequisites

Docker & Docker Compose

Ports 6432 and 6433 free on your host
Using this Docker File


## Using this Docker File
The docker-compose.yaml file in this repository creates a Postgres database named example_db that is replicated between two pgEdge nodes. Before running this file, ensure that you have an installed and running copy of docker with Internet access.

Then, to deploy this example, use the command:
```sh
docker compose up -d
```

## Connecting to example_db with psql
You can interact with the database on each node of your two-node cluster with psql. For convenience, open two terminal windows, and use the following commands to connect to each node. To open a psql session on the first node, run:

```sh
docker compose exec postgres-n1 psql -U pgedge example_db
```

Likewise, to open a `psql` session on the second node, run:
```sh
docker compose exec postgres-n2 psql -U pgedge example_db
```

## Exercising Replication
To demonstrate that the nodes are replicating, you can confirm that a row is replicated from a table on one node to the same table on the other node.

1. Create a table on the first node:
```sh
docker compose exec postgres-n1 psql -U pgedge example_db -c "create table example (id int primary key, data text);"
```
2. Insert a row into our new table on the second node:
```sh
docker compose exec postgres-n2 psql -U pgedge example_db -c "insert into example (id, data) values (1, 'Hello, pgEdge!');"
```
3. See that the new row has replicated back to the first node:
```sh
docker compose exec postgres-n1 psql -U pgedge example_db -c "select * from example;"
```

## Loading the Northwind Sample Dataset
The Northwind sample dataset is a Postgres database dump that you can use to try replication with a more realistic database. To load the Northwind dataset into your pgEdge database, run:

```sh
curl https://downloads.pgedge.com/platform/examples/northwind/northwind.sql | docker compose exec -T postgres-n1 psql -U pgedge example_db
```

Now, try querying one of the new tables from the other node:

```sh
docker compose exec postgres-n2 psql -U pgedge example_db -c "select * from northwind.shippers"
```

## Connecting to example_db from Another Client
If you have psql, pgAdmin, or another Postgres client installed on your host machine, you can use these connection strings to connect to each node:

1. First node: host=localhost port=6432 user=pgedge password=pgedge dbname=example_db.
2. Second node: host=localhost port=6433 user=pgedge password=pgedge dbname=example_db.
   
For example, using psql:

```sh
psql 'host=localhost port=6432 user=pgedge password=pgedge dbname=example_db'
```

## Modifying this Example
Properties specified in a service's environment define the deployment details. You can adjust these settings to customize the deployment.

```sh
environment:
      PGEDGE_USER: pgedge
      PGEDGE_PASSWORD: pgedge
      POSTGRES_USER: pgedge
      POSTGRES_PASSWORD: pgedge
      POSTGRES_DB: example_db
      NODE_NAME: n1
```
1. POSTGRES_USER is the name of the database superuser; the default is pgedge.
2. POSTGRES_PASSWORD is the password associated with the database superuser; the default is pgedge.
3. POSTGRES_DB is the database name; the default is example_db.
4. PGEDGE_USER is the name of the replication user; the default is pgedge.
5. PGEDGE_PASSWORD is the password associated with the replication user; the default is pgedge.
6. NODE_NAME is the logical node name for the node; in our sample file, n1 and n2.

The ports section describes the ports in use by the node:      
```sh
ports:
      - target: 5432
        published: 6432
```



Our published ports are set to 6432 for postgres-n1 and 6433 for postgres-n2.
Our .yaml file includes a clause that defines the creation of each node:
```sh
spock-node-n1:
    content: |-
      #!/usr/bin/env bash
      set -Eeo pipefail
      psql -v ON_ERROR_STOP=1 --username "pgedge" --dbname "example_db" \
        -c "SELECT spock.node_create(node_name := 'n1', dsn := 'host=postgres-n1 port=5432 dbname=example_db user=pgedge password=pgedge');"
```
Note that this configuration only takes effect when the containers are first created. To recreate the database with a new configuration, stop the running example:

```sh
docker compose down
```

And start it again:

```sh
docker compose up -d
```
