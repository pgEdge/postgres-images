# extension-custom-scripts

Scripts for `supautils.extension_custom_scripts_path`, baked into the
`standard` image at `/etc/pgedge/extension-custom-scripts`. supautils
runs these around `CREATE EXTENSION` as the superuser session it
switches to for a privileged install (see `supautils.superuser`), so a
script here runs with superuser privileges, not the installing role's.

The layout follows
[supautils' own convention](https://github.com/supabase/supautils#readme):

```
extension-custom-scripts/
  <extension-name>/
    before-create.sql   # optional, runs before CREATE EXTENSION
    after-create.sql    # optional, runs after CREATE EXTENSION
```

## Rules for a script

Grant access to
[`pg_database_owner`](https://www.postgresql.org/docs/current/predefined-roles.html#PREDEFINED-ROLE-PG-DATABASE-OWNER),
not to a named role. Postgres keeps this role's membership in sync
with whoever owns the database, so the grant follows the database if
it is reassigned to a new owner. A grant to `pg_database_owner` carries
no grant option, so the owner cannot pass it on; anything every role
should have is granted to `PUBLIC` as well.

Grant privileges, and leave ownership with the installing superuser.
The owner of a table can attach a trigger to it, and a trigger runs
with the privileges of whoever writes to the table rather than those
of its owner. Handing a table to `pg_database_owner` therefore lets
the database's owner run code as any superuser that later writes to
that table.

Keep write access narrower than read access. Where a table holds
configuration, `PUBLIC` reads it and only `pg_database_owner` changes
it.

Write each statement against the objects the installed version
actually creates. A script runs inside the `CREATE EXTENSION`
transaction, so an error in it rolls back the install and leaves the
extension uninstallable until the script is fixed. The image test
suite installs every extension that has a script, which is what
catches a name that a new extension version changed.

A script runs only on `CREATE EXTENSION`, not on
`ALTER EXTENSION ... UPDATE`. Objects an upgrade adds are covered only
where a script sets default privileges for them; otherwise the script
has to be run again by hand after the upgrade.

An extension that declares no fixed schema can be installed into any
schema with `CREATE EXTENSION ... SCHEMA`, so its script looks the
schema up from `pg_extension` instead of assuming `public`. supautils'
`@extschema@` substitution cannot replace the lookup: it is set only
when `CREATE EXTENSION` names a schema, and is NULL otherwise. See
`address_standardizer_data_us`.
