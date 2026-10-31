---
title: 'Story 1.7: Web app shell, login and register'
type: 'feature'
created: '2026-10-31'
status: 'done'
baseline_commit: 'fd90625eec0952374f1875ca33f7dc429dce6c86'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
  - '{project-root}/_bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/EXPERIENCE.md'
  - '{project-root}/_bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/DESIGN.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** There is no web app, so nobody can log in, register or see role navigation in a browser (UX-DR1, 3, 5, 6, 7, 20, 21).

**Approach:** Add `web/`, a React 19 + Vite 8 + Tailwind 4 + shadcn/ui SPA. A `web` container serves it with nginx, publishes `127.0.0.1:5173`, and proxies `/api/` to api-gateway, so the browser talks to one origin. It provides login, register, the role-scoped nav and account menu, placeholder home pages, session-expiry handling and the shared UI patterns (skeleton rows, error toast, Money).

## Boundaries & Constraints

**Always:** shadcn/ui components unchanged (DESIGN.md); top nav plus one content column capped at 1152px; copy exactly as in EXPERIENCE.md; role homes Customer `/`, Seller `/seller/orders`, Admin `/admin`; nav: logged out "Products", "Log in"; Customer "Products", "My Orders"; Seller "Seller Orders", "Seller Revenue"; Admin "Ops Dashboard", "Vouchers", "Payment gateway"; account menu with email, role and "Log out"; token and email kept in `sessionStorage`; role and expiry read from the JWT; return-to accepted only as a same-origin path starting with a single `/` and allowed for the logged-in role; error toast "Request failed: {code} — {message}" with the gateway message verbatim; Money renders int64 centavos as right-aligned monospace `R$ 1.234,56`; visible labels on every field, toasts in an `aria-live` region. Test first for every behavior.

**Never:** call any service other than api-gateway; real screens for Products, Orders, Seller or Admin (Stories 1.8 and Epics 2–5); infinite scroll or hover-only actions; product images or brand theming.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Logged out | open `/` | nav "Olist Marketplace", "Products", "Log in" | N/A |
| Login page | open `/login` | email and password with labels, "Demo accounts" hint (2 Customers, 2 Sellers, Admin, demo password), link to `/register` | N/A |
| Login | each demo role | lands on its home; nav shows that role's links and the account menu | wrong password: toast with gateway message, stays on Login |
| Register | new email + password | Customer logged in, lands on `/` | duplicate: toast "Request failed: ALREADY_EXISTS — email already registered" |
| Expired | 401 from any request, or token past `exp` | Login with "Session expired. Log in again."; after login back on the original route | N/A |
| Return-to | `//evil.com`, `https://x`, `/\evil`, another role's route | ignored → role home | N/A |
| Wrong role | Seller opens `/admin` | "You don't have access to this page." with a link home | N/A |
| Loading / failure | a table loads; a request fails | 5 `Skeleton` rows; destructive toast, page stays usable | N/A |
| Money | 123456, 0, 5 | `R$ 1.234,56`, `R$ 0,00`, `R$ 0,05` | N/A |

</frozen-after-approval>

## Code Map

- `web/` -- does not exist yet; `.dockerignore` excludes `web` (adjust so the web image can build from `web/` as its own context).
- `services/api-gateway/internal/routing/routing.go` -- `POST /api/auth/login|register` returning `{access_token, token_type, expires_in}`; errors `{"error":{"code","message"}}`; role groups 401/403.
- `README.md` -- full demo-account list (Story 1.5); pick the first two Customers and Sellers for the hint.
- `deployments/docker-compose.yml` -- add `web` (depends on api-gateway healthy, `127.0.0.1:5173:8080`).
- `tests/scaffold/scaffold_test.go` -- `services`, `infra`, `tools`, `publishedPorts` lists; add `web` as a published, non-Go app.
- `Makefile`, `.github/workflows/ci.yml` -- add web install/lint/test/build targets and a CI job.
- `_bmad-output/implementation-artifacts/deferred-work.md` -- the Login-hint item deferred from Story 1.5 is closed here.

## Tasks & Acceptance

**Execution:**
- [x] `web/` scaffold -- Vite React TS template, Tailwind 4 via `@tailwindcss/vite`, shadcn/ui init (neutral) with `button input label card dropdown-menu skeleton table sonner`; Vitest + Testing Library + jsdom.
- [x] `web/src/lib/` -- `session` (store, JWT decode, expiry), `routes` (role homes, role of a path, `safeReturn`), `api` (gateway fetch, 401 → expired redirect, `ApiError`), `money` (BRL format).
- [x] `web/src/components/` -- `Layout` (nav, account menu, 1152px column), `RequireRole`, `TableSkeleton`, `Money`, `useRequestError` toast helper.
- [x] `web/src/pages/` -- Login (with demo hint), Register, placeholder pages for every nav route and home, NoAccess, NotFound.
- [x] `web/Dockerfile`, `web/nginx.conf` -- build with node, serve with unprivileged nginx on 8080, SPA fallback, `/api/` proxied to `api-gateway:8080`.
- [x] `deployments/docker-compose.yml`, `tests/scaffold`, `Makefile`, `.github/workflows/ci.yml`, `README.md` -- web service and port, scaffold rules, `make web-test`, CI job, README URL.
- [x] Tests -- unit/component tests for the I/O matrix.

**Acceptance Criteria:**
- Given `make up`, when it returns, then `http://127.0.0.1:5173` serves the app and `/api/auth/login` through it reaches the gateway.
- Given the running stack in a browser, when each demo role logs in from the hint, then it lands on its home with its nav, opening another role's route shows the no-access message, and "Log out" returns to the logged-out nav.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD, as in Stories 1.1–1.6.
- Scaffold: `npm create vite` (react-ts, TypeScript 7 so no `baseUrl`), Tailwind 4 via `@tailwindcss/vite`, shadcn/ui `radix-vega` preset with the neutral base; the preset's Inter font was removed so the app uses the system stack (DESIGN.md). npm 10 crashes adding packages to this tree (`edgesOut` arborist bug), so packages were added with `npx npm@11`; `npm ci` works with both.
- `src/lib`: `money` (integer BRL formatting), `routes` (role homes, `roleOfPath`, `canOpen`, `safeReturn`, `loginURL`), `session` (sessionStorage, JWT claim decode, expiry), `api` (`apiFetch`, `ApiError`, 401 handler only when a token was sent).
- `src/session`: `SessionProvider` expires the session on a 401 or at `exp` and redirects to `/login?next=…&expired=1`; log out clears the session in the same transition as the navigation so the role route's guard does not bounce to Login (bug found in the browser, test failed first).
- Pages: Login (demo hint: 2 Customers, 2 Sellers, Admin, password, README pointer — closes the Story 1.5 deferral), Register, placeholders for every nav route, NoAccess, NotFound.
- `web/Dockerfile` (node:24 build, nginx-unprivileged:1.29 serve), `web/nginx.conf` (SPA fallback, `/api/` → `api-gateway:8080`, `/healthz`); compose `web` on `127.0.0.1:5173`; scaffold tests for the port, the single upstream and `depends_on`; `make web-test`; CI `web` job and smoke checks.
- Browser check (`make up`, browser pane): logged-out nav; Login hint; Seller login → `/seller/orders` with Seller nav; `/admin` as Seller → no-access page; account menu (email, role, Log out); an expired stored token → "Session expired. Log in again." → login → back on `/seller/revenue`; log out from a Seller route → Products; Register a new Customer → `/` with Customer nav; same email in upper case → toast "Request failed: ALREADY_EXISTS — email already registered"; Admin login → `/admin` with Admin nav.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge, vgap | A logged-in user's wrong password on Login clears the session and shows "Session expired" | medium | `apiFetch` sent the token and ran the 401 handler on `/api/auth/*`; auth calls now use `auth: false` (tests failed first) | patch |
| 2 | vgap | In-app 401 path and the exp timer untested at app level | medium | New App tests; removing the handler or the timer fails them | patch |
| 3 | blind, edge | Timer expiry on a public route forces Login | low | Now only logs out there (test failed first) | patch |
| 4 | blind, edge | Expired token left in storage keeps "Session expired" on later visits | low | Cleared when detected at load (test failed first) | patch |
| 5 | blind | Expiry handler re-registered on every navigation | low | Location read through a ref | patch |
| 6 | edge | `safeReturn` misses case variants and trailing slashes (`/ADMIN`, `/login/`) | low | Router matches case-insensitively; normalised (tests failed first) | patch |
| 7 | blind, edge | Empty 2xx body throws SyntaxError; caller abort reported as UNAVAILABLE | low | Handled (tests failed first) | patch |
| 8 | edge | `logIn` returning null leaves the form silent | low | Now a toast | patch |
| 9 | edge | setTimeout overflow for far-future `exp` | low | Delay clamped | patch |
| 10 | edge | nginx caches the gateway IP; a recreated gateway gives 502 | medium | `resolver 127.0.0.11` with a variable upstream; `nginx -t` passes; scaffold check updated | patch |
| 11 | blind | Missing security headers in nginx | low | `nosniff`, `X-Frame-Options DENY`, `Referrer-Policy` added | patch |
| 12 | blind | Toasts follow OS dark mode on a light page | low | Seen in the browser check; Toaster now `theme="light"` | patch |
| 13 | blind | Dev proxy `/api` vs nginx `/api/`; autocomplete keyed on button label | low | Aligned; explicit `passwordAutoComplete` prop | patch |
| 14 | blind | Node version not pinned; CI web job uncached | low | `engines`, `.nvmrc`, setup-node cache | patch |
| 15 | blind | Logged-in users can open Login/Register | low | Harmless (Register makes a separate Customer); redirect races the post-login navigation | reject |
| 16 | blind | Demo hint can drift from the README | low | Dataset is fixed; README is the full list | reject |
| 17 | blind | Nav has no phone layout; no inline field errors | low | Desktop-only, toast errors per DESIGN/EXPERIENCE | reject |
| 18 | edge | Money with NaN, > 2^53, or −0.5 | low | Inputs are int64 centavos from the gateway | reject |
| 19 | edge | Missing static file falls back to index.html | low | Hashed assets under `/assets/`; no real request misses | reject |
| 20 | edge | Spec names a `useRequestError` hook; code has `toastRequestError` | low | Fix is a spec edit | reject |
| 21 | blind | Diff omits generated shadcn files and lockfile | false | Deliberately excluded; reviewed separately (theme issue fixed) | reject |

## Design Notes

- One origin (nginx proxy) means no CORS and the browser never learns service addresses.
- The JWT carries no email, so the email typed at login/register is stored with the token for the account menu.
- `web` is not a Go module: scaffold tests treat it as a published app outside `SERVICES`.

## Verification

**Commands:**
- `cd web && npm ci && npm run lint && npm test && npm run build` -- expected: exit 0.
- `make build && make test` -- expected: exit 0 (scaffold rules updated).
- `make up` -- expected: exit 0 with `web` running.

**Manual checks (if no CLI):**
- In the browser pane on `http://127.0.0.1:5173`: log in as each demo role, check nav, home, no-access page, log out; register a new Customer; force an expired session.
