# Resume Note: First-Screen Cutover & CI Deployment (2026-09-28)

## 1. What Shipped & Is Live in Production
- **Add Screen (US-013)**: Live at `https://scry.study/add` behind Cloudflare Access.
  - Single text field with JetBrains Mono / Merriweather typography.
  - Optional photo picker with live file chip, preview thumbnail, and accessible removal (`aria-label="Remove photo"`).
  - Strict privacy boundary: typed or shared URLs remain private `my_text`; no web research is initiated by inference.
  - Dynamic byte counter with 90% near-limit warning, over-limit error state, and Ctrl/Cmd+Enter shortcut.
  - Zero horizontal overflow at 320 CSS px (`scrollWidth === clientWidth === 320`).
- **Production Container**: Application `scry-app-container` (ID `a035bc19-23be-4bba-a6a9-ec114b8af995`) is deployed on Cloudflare Containers with image `registry.cloudflare.com/b069014f6a46558ea9146fb6c4ff8f6c/scry-app-container:8c916460` (digest `sha256:8dfdb46c10e82303b06e465fc13d98da02312a8f637271ab02793bf94010d1c1`), carrying tested release binary `scry 726ee69` (SHA-256 `d11e7bc0136c52bc3d0e6034c5a25c373fa3d9d8499f51d705fc8f8b248dd59a`).
- **Production Worker & Triggers**: `scry-app-host` version `795205b8-8609-4178-9f74-809fcad7440d` with committed `24h` idle policy, custom domains `scry.study`, `www.scry.study`, `scry.mistystep.io`, and UTC cron `0 */12 * * *`.

## 2. Deploy Job State
- The `deploy` job lives in its own PR (#210), not in this change. It runs on `push` to `master` only after `ci`, `foundation`, and `story-walk` pass.
- It downloads the tested `scry-linux-amd64-<sha>` artifact from the same run, copies `scry` into `deploy/cloudflare-hosting/`, and runs `npx wrangler deploy --env production` with repo secret `CLOUDFLARE_API_TOKEN`. No release wrappers.
- Known risk: `docs/runbook.md` requires quiescing the container before an image rollout. A deploy whose binary differs from the running one replaces the instance without that quiesce; the entrypoint takes a final backup on SIGTERM, but confirm the R2 snapshot after any rollout.

## 3. Tokens: where they live and how they are minted
- **Master token**: pass `workstation/CLOUDFLARE_API_TOKEN` (Misty Step account token including Account API Tokens Write). It mints and edits tokens. It never goes into CI, a repo secret, or a Worker.
- **Deploy token**: pass `workstation/SCRY_CLOUDFLARE_DEPLOY_TOKEN`, mirrored to the GitHub repo secret `CLOUDFLARE_API_TOKEN` on `misty-step/scry`. Token name `scry-github-deploy-20260928-v2`, id `83f6c2bc116bf54a1a60b57ed9f78c0d`.
- **Mint or edit** (account-owned tokens, not `/user/tokens`, which returns 403 for every held credential): run with `pass-env run -e MASTER=workstation/CLOUDFLARE_API_TOKEN`, then `POST /client/v4/accounts/{account}/tokens` to create or `PUT .../tokens/{id}` to replace the policy list. List permission-group ids at `GET .../tokens/permission_groups`. Verify with harmless reads using the new value, and only then store it: `printf %s "$VALUE" | pass insert -m -f workstation/SCRY_CLOUDFLARE_DEPLOY_TOKEN` and `gh secret set CLOUDFLARE_API_TOKEN -R misty-step/scry`. Store the value with no trailing newline: `pass-env` uses the whole plaintext. Never print it.
- **Deploy token grants** (account `b069014f6a46558ea9146fb6c4ff8f6c`): Workers Scripts Write, Workers Containers Write, Workers R2 Storage Read, Workers R2 Storage Metadata Read, Account Settings Read; R2 Bucket Item Read on `scry-go-recovery` and `scry-go-recovery-staging`; on zones `scry.study` (`f7be06f7426d5532921cc4348746a2b8`) and `mistystep.io` (`82511c89ffe04f84cb6091ba0a803109`, which hosts `scry.mistystep.io`): Zone Read, Workers Routes Write, DNS Read.
- **If a deploy fails on a custom-domain step**, the first suspect is DNS Write on the zone, deliberately not granted; add it via the PUT above and record why.

## 4. Next Redesigned Screens in Order
Follow the Ink notebook / JetBrains Mono typography and layout system established on Add:
1. **Stream (`/`)**:
   - Single-question review card with Merriweather reading serif.
   - Held graded feedback state, deliberate "Next" progression.
   - Accessible recall inputs (choice and short-v1 text).
   - "Look it up" drawer and concept note navigation.
2. **Map (`/map`)**:
   - Goal sections and prerequisite-ordered concept cards.
   - Star chart / constellation decorative ornament.
   - Recall estimates, status labels ("Due", "Learning", "Mastered"), and pause/focus controls.
3. **Concept Details (`/concepts/{id}`)**:
   - Teachable concept notes, citations, related concepts.
   - Separate unaided vs assisted vs missed tallies.
   - Practice action.
4. **History & Settings (`/history`, `/settings`)**:
   - Past reviews timeline.
   - Backup status and daily generation budget usage.

## 5. Status and Exact Next Step
- Done 2026-09-28: PR #209 (Add screen) and PR #210 (deploy job) merged. Master run 36502757181 passed `ci`, `foundation`, `story-walk`, then `deploy` with the scoped token; Worker version `15492eef-d833-43de-92fc-e42ddf7cfe28`, container application version 8, image digest `sha256:6b10c8e8dee0c47d4dd664ee5937dcb6995d660ad10d6933f3e2c8e26b91ce26`. The three custom domains deployed without DNS Write. Old token `scry-github-deploy-20260928` was revoked after that run.
- Not yet walked: `https://scry.study/add` behind Access on a real phone with the keyboard open, on the CI-built binary. The container was inactive after the rollout and restores from R2 on the next owner visit; check that first visit and the R2 snapshot.
- Next: start the Stream screen with the Opus 5.5 max designer, per Section 4. If a later deploy fails with 401/403, adjust the deploy token per Section 3; never put the master token in CI.
