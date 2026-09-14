# extension-custom-scripts

Scripts for `supautils.extension_custom_scripts_path`, baked into the
`standard` image at `/etc/pgedge/extension-custom-scripts`. supautils
runs these around `CREATE EXTENSION`, as the superuser session it
already switches to for a privileged install (see
`supautils.superuser`), so a script here can assume superuser
privileges, not just the installing role's own.

Layout, per [supautils' own convention](https://github.com/supabase/supautils#readme):

```
extension-custom-scripts/
  <extension-name>/
    before-create.sql   # optional, runs before CREATE EXTENSION
    after-create.sql    # optional, runs after CREATE EXTENSION
```

This image is not exclusive to any one deployment's role model, and an
extension can be installed into any database, owned by whatever role
happens to own it. A script granting access to "the role that should be
able to use this" should grant to
[`pg_database_owner`](https://www.postgresql.org/docs/current/predefined-roles.html#PREDEFINED-ROLE-PG-DATABASE-OWNER),
not a hardcoded role name: Postgres automatically maintains membership
in this predefined role to match whoever currently owns the database,
so the grant keeps working even if that database is later reassigned
to a different owner, and needs no assumption about what the owner is
named. See `pg_cron`'s `after-create.sql` for the pattern. A script
that blocks an install outright regardless of role (see `lolor`'s
`before-create.sql`) has no such grant to make in the first place,
refusing a broken install is correct for every consumer of this image.
