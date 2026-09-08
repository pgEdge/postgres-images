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

This image is not exclusive to any one deployment's role model. A
script that assumes a specific role exists (e.g. `app`) must check for
it first and no-op otherwise, so `CREATE EXTENSION` still succeeds for
a consumer of this image who has no such role. See `pg_cron`'s
`after-create.sql` for the pattern. A script that blocks an install
outright regardless of role (see `lolor`'s `before-create.sql`) needs
no such check: refusing a broken install is correct for every
consumer of this image, not a role-model-specific decision.
