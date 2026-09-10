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
named. See `pg_cron`'s `after-create.sql` for the pattern.

A script here only runs in a session that has `supautils` loaded.
Every privileged (allowlisted) extension's `before-create.sql` checks
`session_user` against `supautils.privileged_role` directly, rather
than relying on only the privileged role's session ever loading
`supautils` in the first place: `supautils.privileged_extensions`
itself has no concept of "who is asking", it only checks the
extension name, so restricting installs to one role has always
depended on `supautils` being loaded cluster-wide
(`shared_preload_libraries`) and this check being the thing that
actually enforces who gets to use it, not session scoping.

`session_user` stays the role that actually authenticated for the
whole session, even once supautils switches the acting role to
install the extension, so the check holds regardless of that
elevation. See `postgis/before-create.sql` for the plain case and
`lolor/before-create.sql` for one that layers an extension-specific
requirement on top of the same role check.
