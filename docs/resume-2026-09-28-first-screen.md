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
- `.github/workflows/ci.yml` has a standard `deploy` job (commit `1a26fdf`).
- Triggers on `push` to `master` only after `ci`, `foundation`, and `story-walk` succeed.
- Reuses the exact tested Linux binary artifact (`target/ci-release/scry`) from the `ci` job.
- Runs `npx wrangler deploy --env production` directly from `deploy/cloudflare-hosting`. No release wrappers or custom backup scripts.

## 3. Token Architecture & Automated Minting
Master token lives in pass at `workstation/CLOUDFLARE_API_TOKEN`. It is a broad Misty Step account token carrying `Account API Tokens: Write`, `Workers Containers: Write`, `Workers R2 Storage: Write`, and zone-level `Workers Routes: Write` and `DNS: Write`.

Cloudflare uses **Account-Scoped API Tokens** (not user-level `/user/tokens`). Account tokens are queried and minted via:
```http
POST https://api.cloudflare.com/client/v4/accounts/{account_id}/tokens
Authorization: Bearer <workstation/CLOUDFLARE_API_TOKEN>
```

Using this path, a least-privilege token was minted:
- **Name**: `scry-github-deploy-20260928-v2` (ID: `83f6c2bc116bf54a1a60b57ed9f78c0d`)
- **Account Permissions** (`b069014f6a46558ea9146fb6c4ff8f6c`):
  - `Workers Containers: Write` (`bdbcd690c763475a985e8641dddc09f7`)
  - `Workers Scripts: Write` (`e086da7e2179491d91ee5f35b3ca210a`)
  - `Workers R2 Storage: Read` (`b4992e1108244f5d8bfbd5744320c2e1`)
  - `Workers R2 Storage Metadata: Read` (`dc1beb502339482da2515d6e146ca1ac`)
  - `Workers R2 Storage Bucket Item: Read` (`6a018a9f2fc74eb6b293b0c548f38b39` on `scry-go-recovery*`)
  - `Workers KV Storage: Write` (`f7f0eda5697f475c90846e879bab8666`)
  - `Account Settings: Read` (`c1fde68c7bcc44588cbb6ddbc16d6480`)
- **Zone Permissions** (`scry.study` / `82511c89ffe04f84cb6091ba0a803109`):
  - `Zone: Read` (`c8fed203ed3043cba015a93ad1616f1f`)
  - `Workers Routes: Write` (`28f4b596e7d643029c524985477ae49a`)
  - `DNS: Read` (`82e64a83756745bbbb1c9c2701bf816b`)
  - `DNS: Write` (`4755a26eedb94da69e1066d98aa820be`)
- **Verification**: Verified live on container registry credentials POST (HTTP 201), scripts read (HTTP 200), R2 buckets (HTTP 200), zone read (HTTP 200), DNS records (HTTP 200), and routes (HTTP 200).
- **Pass & Secret**: Decrypted value stored into pass `workstation/SCRY_CLOUDFLARE_DEPLOY_TOKEN` and updated as GitHub repository secret `CLOUDFLARE_API_TOKEN` on `misty-step/scry`.
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

## 5. Exact Next Step and Unblocking
1. **First PR**: Carries Add screen (US-013) and this resume note to `master`.
2. **Deploy-Job PR**: Carries `.github/workflows/ci.yml` deploy job. Merged once first PR is on master.
3. **Green Master Proof**: CI runs `ci`, `foundation`, `story-walk`, then `deploy` using the least-privilege token.
4. **Token Revocation**: Once green master deploy succeeds, the previous token (`d99708f4127a5683bf5eb184cbc17116`) is revoked via `DELETE /accounts/{account_id}/tokens/d99708f4127a5683bf5eb184cbc17116`.
