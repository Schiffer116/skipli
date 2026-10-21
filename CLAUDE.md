# E2E testing

Drive the real app against Development, signed in as the test user
`e2e@e2e.dev.skipli.schifferarchitecture.com` (never the user's own account).
Its mail lands in S3 (`infra/e2e.tf`); `scripts/e2e-code` reads the code, so
never ask the user for one.

1. API (`api/Makefile` reads the pool ids from `terraform -chdir=infra output`,
   so init `infra` with `-backend-config=env/dev.s3.tfbackend` first). Pick
   free ports, other agents run their own:
   `AWS_PROFILE=iamadmin-general PORT=3417 make -C api run`
2. UI: `cd ui && pnpm install && pnpm build && API_URL=http://localhost:3417 pnpm preview --port 4417 --strictPort`
3. Browser, with this env on every `chrome-devtools-axi` call (the profile is
   outside `/tmp` so a sign-in lasts its 30 days; only Chromium is installed):

   ```sh
   export CHROME_DEVTOOLS_AXI_USER_DATA_DIR=~/.cache/skipli-e2e/chrome-profile \
     CHROME_DEVTOOLS_AXI_SESSION=skipli-e2e \
     CHROME_DEVTOOLS_AXI_EXECUTABLE_PATH=/usr/bin/chromium
   chrome-devtools-axi open http://localhost:4417
   ```

4. Signed out (page is `/login`): fill the email, `since=$(date +%s)`,
   `press Enter`, then fill `$(scripts/e2e-code "$since")` and `press Enter`.

Google sign-in can't be automated: test it with the user, and only on UI port
4417, the one local origin the user pool allows (`local_app_urls` in
`infra/env/dev.tfvars`).

chrome-devtools-axi quirks:

- `open` on a freshly launched browser fails with "No page is currently
  selected"; `selectpage 2` (the app's tab) fixes it.
- After `fill`, `press Enter` to submit.
- Offline: `emulate --network Offline`; back online: `emulate --network "Fast 4G"`.
