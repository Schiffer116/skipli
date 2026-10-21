# E2E testing

Drive the real app against Development, signed in as your own test user
`e2e-<your worktree or Division name>@e2e.dev.skipli.schifferarchitecture.com`
(never the user's own account, never another agent's; the API allows one
sign-in mail per address per minute, so a shared address makes parallel agents
fail). Mail for any address at that domain lands in S3 (`infra/e2e.tf`);
`scripts/e2e-code` reads the code, so never ask the user for one. Each address
is its own user, so its boards are yours alone: delete test boards when done.

1. API (`api/Makefile` reads the pool ids from `terraform -chdir=infra output`,
   so init `infra` with `-backend-config=env/dev.s3.tfbackend` first). Pick
   free ports, other agents run their own:
   `AWS_PROFILE=iamadmin-general PORT=3417 make -C api run`
2. UI: `cd ui && pnpm install && pnpm build && API_URL=http://localhost:3417 pnpm preview --port 4417 --strictPort`
3. Browser, with this env on every `chrome-devtools-axi` call (`<name>` as in
   your address, so agents don't share a profile; it is outside `/tmp` so a
   sign-in lasts its 30 days; only Chromium is installed):

   ```sh
   export CHROME_DEVTOOLS_AXI_USER_DATA_DIR=~/.cache/skipli-e2e/<name> \
     CHROME_DEVTOOLS_AXI_SESSION=skipli-e2e-<name> \
     CHROME_DEVTOOLS_AXI_EXECUTABLE_PATH=/usr/bin/chromium
   chrome-devtools-axi open http://localhost:4417
   ```

4. Signed out (page is `/login`): fill the email, `since=$(date +%s)`,
   `press Enter`, then fill `$(scripts/e2e-code "$since" e2e-<name>)` and
   `press Enter`. On a 429 from `/api/auth/send`, wait out the cooldown (a
   minute) instead of retrying.

chrome-devtools-axi quirks:

- `open` on a freshly launched browser fails with "No page is currently
  selected"; `selectpage 2` (the app's tab) fixes it.
- After `fill`, `press Enter` to submit.
- Offline: `emulate --network Offline`; back online: `emulate --network "Fast 4G"`.

# Architecture diagram

When a change alters the architecture (`infra/` resources or how they
connect), update `infra/terravision.yml` and regenerate `infra/architecture.png`
with `make -C infra diagram` (plans only, applies nothing) in the same PR.
