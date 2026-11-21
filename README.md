# CholoBangladesh API — User Module

A Go (Gin) backend for a tourist platform. Tourists (`user`) browse and book **verified local guides** (`guide`).

## Quick start

```bash
# 1) Full stack in Docker (Postgres + Redis + API)
docker compose up -d --build
#    API      → http://localhost:8080
#    Swagger  → http://localhost:8080/swagger/index.html
#    Health   → /health (liveness), /ready (DB + Redis)

# 2) Or: databases in Docker, API locally with hot reload (recommended for development)
docker compose stop api               # free port 8080 if the API container was running
docker compose up -d postgres redis   # service names are "postgres" and "redis" (there is no "db")
air                                   # or: go run ./cmd/server
```

**Daily workflow (DB in Docker + API with air):**
```bash
docker compose up -d postgres redis      # once, at the start of the day
air                                      # rebuilds on save; Ctrl+C to stop
docker compose --profile tools up -d     # optional: Adminer :8081, RedisInsight :5540
```
* air reads `.env` (`DATABASE_URL` → `localhost:5433`, `REDIS_ADDR` → `localhost:6379`), so it talks to the Docker databases.
* Every build regenerates Swagger as well, so new annotations show up right after you save.
* Seed and reset commands work with air too: `go run ./cmd/manage seed`. After `reset-db` or `fresh`, restart air (Ctrl+C, then `air`).

**Troubleshooting (Windows)**
| Problem | Fix |
|---|---|
| `no such service: db` | Use `postgres` / `redis` (see `docker compose config --services`) |
| `bind: address already in use` on :8080 | `docker compose stop api` (the Docker API container and air cannot both use 8080) |
| `http://localhost:8080` hangs but `http://127.0.0.1:8080` works | A stale Docker/WSL port relay is holding `localhost`. Run `docker compose rm -f api`; if it still hangs, restart Docker Desktop (or run `wsl --shutdown`) |
| `failed to parse .air.toml` / `'&&' is not a valid statement separator` | Already fixed in `.air.toml` (air on Windows runs commands through PowerShell 5.1) |

* Postgres is published on **127.0.0.1:5433**, so it does not clash with a local Postgres on 5432.
* Redis is published on 127.0.0.1:6379 and is password protected.
* Migrations run automatically at startup (`MIGRATE_ON_START=true`, SQL files in `internal/platform/database/migrations`).
* The admin account is created from `ADMIN_EMAIL` / `ADMIN_PASSWORD` at startup.
* **Dev mode:** with `SMS_ENABLED=false` / `EMAIL_ENABLED=false`, OTP codes are printed in the server log (`docker compose logs api | grep otp=`) instead of being sent. In production this is refused at startup.

Regenerate Swagger after changing annotations:
```bash
swag init -g cmd/server/main.go -o docs --md docs/markdown --parseInternal
```
`cmd/server/main.go` imports `_ "cholobangladesh-server/docs"` so the generated spec is registered.

---

## Viewing the data (Postgres & Redis)

> ⚠️ `http://127.0.0.1:5433` and `http://127.0.0.1:6379` **cannot be opened in a browser**. They are database protocols, not HTTP, so the browser shows `ERR_EMPTY_RESPONSE`. That is normal. Use the tools below.

### Option 1: web UI in the browser (recommended)
```bash
docker compose --profile tools up -d      # starts Adminer + RedisInsight
```

| What | URL | Login |
|---|---|---|
| **Postgres** (Adminer) | http://localhost:8081 | System: `PostgreSQL` · Server: `postgres` · Username: `cholo` · Password: `POSTGRES_PASSWORD` from `.env` · Database: `cholobangladesh` |
| **Redis** (RedisInsight) | http://localhost:5540 | Click the **cholobangladesh** connection; no login needed (it is pre-configured) |

* In Adminer, click a table such as `users`, `user_profiles` or `guide_profiles`, then **Select data**. Use **SQL command** to run your own queries.
* In RedisInsight, browse keys: `rl:*` = rate limits, `cb:user:otp:*` = OTPs (stored hashed), `cb:user:login:*` = lockouts, `cb:user:authstate:*` = auth cache. You can also delete a key by hand to unlock something.
* Inside Docker the host names are `postgres` and `redis`, **not** `localhost`.
* Stop the tools with `docker compose --profile tools stop adminer redisinsight`.

### Option 2: desktop app (DBeaver, pgAdmin, TablePlus, Another Redis Desktop Manager)
| | Host | Port | User | Password | Database |
|---|---|---|---|---|---|
| Postgres | `localhost` | **5433** | `cholo` | `POSTGRES_PASSWORD` | `cholobangladesh` |
| Redis | `localhost` | 6379 | — | `REDIS_PASSWORD` | db `0` |

The VS Code extensions *PostgreSQL* (by Chris Kolkman / Microsoft) and *Database Client* work with the same settings.

### Option 3: terminal
```bash
docker compose exec postgres psql -U cholo -d cholobangladesh
#   \dt                       list tables
#   select u.email, u.phone, u.role, p.full_name from users u join user_profiles p on p.user_id = u.id;
#   \q                        quit

docker compose exec redis sh -c 'redis-cli -a "$REDIS_PASSWORD" --no-auth-warning'
#   keys *                    all keys
#   keys rl:*                 rate-limit counters
#   ttl <key>                 seconds left
```

---

## Seed data — default accounts

```bash
docker compose exec api manage seed        # inside Docker (any shell)
go run ./cmd/manage seed                   # or locally (uses .env)
```
Running it again is safe: accounts that already exist are skipped. **Every seeded account uses the password `Password123`.**

| Role | Name | Login (email) | Login (phone) | Notes |
|---|---|---|---|---|
| admin | Super Admin | `admin@cholobangladesh.com` | — | Can review KYC and suspend users |
| user | Emma Watson | `emma@example.com` | — | Tourist 🇬🇧 London, onboarding done |
| user | John Carter | `john@example.com` | — | Tourist 🇺🇸 New York |
| user | Tanvir Ahmed | `tanvir@example.com` | — | Local tourist 🇧🇩 Chattogram |
| guide | Rahim Uddin | `rahim.guide@example.com` | `01711000001` | ✅ verified · Sundarbans/Khulna · ৳900/h · ★4.9 |
| guide | Nusrat Jahan | `nusrat.guide@example.com` | `01811000002` | ✅ verified · Sylhet tea gardens · ৳700/h · ★4.8 |
| guide | Arif Hossain | `arif.guide@example.com` | `01911000003` | ✅ verified · Old Dhaka history · ৳600/h · ★4.7 |

Guides can log in with either their email or their phone. All three appear in `GET /api/v1/guides`.

> The bootstrap admin from `.env` (`ADMIN_EMAIL` / `ADMIN_PASSWORD`) is a separate account, created automatically at startup. It exists for production; use the seeded admin for development.

---

## Development OTP (frontend friendly)

In `.env` (development only):
```env
DEV_OTP_CODE=123456     # every OTP is always 123456
DEV_EXPOSE_OTP=true     # the send-otp response also returns the code
```
```json
{ "success": true, "data": { "channel": "email", "destination": "ne****@example.com",
  "expires_in_seconds": 300, "resend_after_seconds": 60, "dev_otp": "123456" } }
```
* The frontend can auto-fill `data.dev_otp` in dev builds. Do not rely on it in production code, because the field is absent there.
* With `SMS_ENABLED=false` / `EMAIL_ENABLED=false` nothing is actually sent; the code is also printed in the log (`docker compose logs api | grep otp=`).
* If `APP_ENV=production` and either option is set, **the server refuses to start**, so it cannot leak by accident.
* Want to test the real flow? Leave `DEV_OTP_CODE` empty and set `DEV_EXPOSE_OTP=false`.
* For **passwordless login, forgot password and contact verification**, `dev_otp` only appears when the account exists (unknown accounts get no code by design).

---

## Reset commands (rate limits, lockouts, database, Redis)

Use `docker compose exec api manage <cmd>` (Docker) or `go run ./cmd/manage <cmd>` (local). The Makefile has shortcuts such as `make seed` and `make fresh`.

| Problem / goal | Command | What it clears |
|---|---|---|
| `RATE_LIMITED` (429) | `manage reset-ratelimit` | All rate-limit counters (`rl:*`) |
| `ACCOUNT_TEMPORARILY_LOCKED`, `OTP_COOLDOWN`, `OTP_LIMIT_REACHED` | `manage reset-locks` | Login lockouts, OTP blocks, resend cooldowns, hourly OTP caps, SMS-per-IP caps |
| Unlock only one account | `manage unlock emma@example.com` or `manage unlock 01711000001` | Lockouts for that email or phone |
| Clear everything in Redis | `manage reset-redis` | OTPs, locks, rate limits, caches. Logins still work because sessions live in Postgres. |
| Empty the database | `manage reset-db` then **`docker compose restart api`** | Drops all tables and re-runs migrations |
| Start over with demo data | `manage fresh` then **`docker compose restart api`** | reset-db + reset-redis + seed |
| Delete everything, including Docker volumes | `docker compose down -v && docker compose up -d --build` then `manage seed` | Postgres and Redis data volumes |

* `reset-db`, `fresh` and `reset-redis` **refuse to run when `APP_ENV=production`** (add `--force` only if you really mean it).
* After `reset-db` or `fresh` you must restart the API. The running server holds cached database connections that still point to the dropped tables, so skipping the restart gives `INTERNAL_ERROR`.
* Git Bash on Windows: always write `manage`, not `/manage`. Git Bash turns `/manage` into a Windows path.

---

## Rate limits & lockouts explained

There are **two different kinds** of 429 errors. Check `error.code` to tell them apart:

**1. `RATE_LIMITED`: too many requests (it counts *requests*, regardless of who sends them)**

| Bucket | Limit (default `.env`) | Key | Applies to |
|---|---|---|---|
| global | `RATE_LIMIT_GLOBAL_PER_MIN=120` / min | IP | Every `/api/v1/*` request |
| auth | `RATE_LIMIT_AUTH_PER_MIN=20` / min | IP | Every `/auth/*` request |
| otp-send | `RATE_LIMIT_OTP_SEND_PER_10MIN=5` / 10 min | IP | All send-otp and forgot-password requests |
| upload | `RATE_LIMIT_UPLOADS_PER_HOUR=20` / hour | user | Avatar and KYC uploads |
| sensitive | 10 / 10 min | user | Change password, delete account |

These are **fixed windows**: when the window ends the counter resets on its own. `retry_after_seconds` tells you how long that takes; for example 148 s means the 10-minute OTP window still has 148 s left. Every response carries `X-RateLimit-Limit` and `X-RateLimit-Remaining` headers. During development all requests come from one IP (localhost), so these limits are reached quickly. Use `manage reset-ratelimit`, or raise the numbers in `.env` and run `docker compose up -d api`.

**2. Account/OTP protection (counts *failures* for one email or phone)**

| Code | When | How long | Setting |
|---|---|---|---|
| `INVALID_CREDENTIALS` + `details.remaining_attempts` | Wrong password | — | — |
| `ACCOUNT_TEMPORARILY_LOCKED` | `USER_MAX_FAILED_ATTEMPTS` (5) wrong passwords within `USER_FAILED_ATTEMPT_WINDOW` (15m) | `USER_BLOCK_TIME` | Also applies after 30 failed logins from one IP in an hour |
| `OTP_INVALID` + `remaining_attempts` | Wrong OTP | — | — |
| `ACCOUNT_TEMPORARILY_LOCKED` | `OTP_MAX_ATTEMPTS` (5) wrong codes | `USER_BLOCK_TIME` | That email or phone cannot request or verify OTPs |
| `OTP_COOLDOWN` | Resend within `OTP_RESEND_COOLDOWN` (60s) | Up to 60 s | — |
| `OTP_LIMIT_REACHED` | More than `OTP_MAX_PER_HOUR` (5) codes to one target, or more than `SMS_MAX_PER_IP_PER_DAY` SMS from one IP | Up to 1 h / 24 h | — |

Passwordless OTP login and password reset also clear the password lockout, because the user has proven they own the email or phone.

**Frontend handling:** on any 429, disable the button and show a countdown from `error.retry_after_seconds`.

---

## Architecture (clean architecture)

```
cmd/server/main.go                 bootstrap, swagger general info, graceful shutdown
internal/
  config/                          env loading + validation (fails fast)
  http/router.go                   gin engine, global middleware, /health /ready /swagger
  http/middleware/                 request-id, logger, recovery, security headers, body limit,
                                   rate limit, JWT auth + session check, role, captcha
  platform/                        adapters to the outside world
    database/  (+ migrations/*.sql)   cache/   mailer/ (SMTP)   sms/ (BulkSMSBD)
    storage/ (Cloudinary)   oauth/ (Google/Apple ID-token)   captcha/ (Turnstile)
  modules/user/
    domain/       entities, business rules (onboarding, age), errors, PORTS (interfaces)
    service/      use cases — depends only on domain ports
    repository/   Postgres (GORM) + Redis implementations of the ports
    dto/          request/response contracts (what the frontend sees)
    handler/      HTTP layer + swagger annotations
    module.go     wiring + routes
pkg/              reusable: apperror, response, token, password, secure, phone, ratelimit, validation, imageutil
```
Dependency direction: `handler → service → domain ← repository`. To add a new module (booking, tour, ...), create `internal/modules/<name>` with the same layout and pass it to `NewRouter`.

## Data model: why separate tables?

| Table | Purpose |
|---|---|
| `users` | **Identity and auth only**: role, status, email/phone and when each was verified, password hash, token_version. It is read on every login and auth check, so it stays narrow. |
| `user_profiles` | Personal info shared by both roles (1:1): name, avatar, bio, gender, DOB, country, city, language, WhatsApp. |
| `guide_profiles` | Marketplace data only guides have (1:1): headline, about, languages[], service_areas[], specialties[], rates, rating, verification_status. GIN indexes for search. |
| `guide_verifications` | **History** of KYC submissions: NID encrypted with AES-256-GCM, document public IDs (private Cloudinary), admin review fields. Only one pending submission is allowed per guide. |
| `auth_identities` | Linked Google/Apple accounts. |
| `user_sessions` | One row per device. Stores the refresh token SHA-256 and the previous hash, which is used to detect reuse. |
| `audit_logs` | Security events: logins, lockouts, KYC views and decisions, status changes. |

A single `users` table holds both roles (there is no separate guide-account table), so login, tokens and sessions work the same way for everyone. Role-specific data lives in the 1:1 profile tables, which keeps the hot auth table small and avoids many columns that are NULL for tourists.

## Sign-up flows

**Tourist** (no SMS cost): email OTP → verify → complete. Google or Apple sign-in takes one step.
**Guide:** BD mobile OTP → verify → complete → personal profile + avatar → guide profile → NID + selfie → admin approves → listed publicly.

The frontend reads `user.onboarding.next_step` after every login to decide which screen to show. See Swagger for request and response examples and every error code.

## Security layers

| Layer | What it does |
|---|---|
| Network/HTTP | Security headers (CSP, HSTS in prod, nosniff, frame deny), strict CORS, body size limits, server timeouts, `TRUSTED_PROXIES` so a forged `X-Forwarded-For` cannot bypass IP limits |
| Rate limiting (Redis) | Global per IP · auth per IP · OTP-send per IP · uploads per user · sensitive actions per user |
| Bot protection | Optional Cloudflare Turnstile on OTP, login and forgot-password requests (`X-Captcha-Token`) |
| OTP | Stored as an HMAC (never plain), bound to its purpose, single use, 5 min TTL, 60 s resend cooldown, hourly cap per target, `OTP_MAX_ATTEMPTS` wrong tries then the target is blocked for `USER_BLOCK_TIME`. SMS goes to BD numbers only, with a per-IP daily SMS cap (protects against SMS pumping and toll fraud). |
| Login | Lockout after `USER_MAX_FAILED_ATTEMPTS` (in `USER_FAILED_ATTEMPT_WINDOW`) for `USER_BLOCK_TIME`; per-IP block against credential stuffing; constant-time response for unknown users; bcrypt cost 12 |
| Enumeration | OTP login and forgot-password give the same response whether or not the account exists |
| Tokens | 15 min HS256 JWT (issuer, audience and algorithm pinned) + opaque 30 day refresh token, rotated on each use; reuse of an old token revokes the session. Logout, logout-all, password reset and suspension take effect immediately (session blacklist + `token_version`, cached in Redis). |
| Data | NID encrypted with AES-256-GCM; KYC images stored as private Cloudinary assets that admins see through 10-minute signed URLs (every view is audited); public guide API never shows phone or email |
| Uploads | File type detected from content, image decoded, pixel limit (decompression bombs), size limit |
| Container | distroless, non-root, read-only FS, all capabilities dropped, no-new-privileges, DB and Redis bound to localhost |

## Production checklist
* `APP_ENV=production`, `SMS_ENABLED=true`, `EMAIL_ENABLED=true`, real `CORS_ORIGINS`, `TRUSTED_PROXIES` = your load balancer.
* Generate new `JWT_SECRET`, `OTP_HMAC_SECRET`, `DATA_ENCRYPTION_KEY`, DB and Redis passwords, and keep them in a secret manager.
* `CAPTCHA_ENABLED=true` + `TURNSTILE_SECRET_KEY`; set `GOOGLE_CLIENT_IDS` and `APPLE_CLIENT_IDS`.
* Run behind TLS (nginx/Caddy/Cloudflare). Set `SWAGGER_ENABLED=false` if the docs must not be public.
* Back up Postgres, and back up `DATA_ENCRYPTION_KEY` separately (without it stored NIDs cannot be decrypted).
