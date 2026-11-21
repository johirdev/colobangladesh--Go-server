Backend API for **CholoBangladesh**, a tourist platform where travellers find and book verified local guides.

## Roles
| Role | Who | How they sign up |
|---|---|---|
| `user` | Tourist (foreign or local) | **Email OTP** (3 steps), or **Google / Apple** (1 step). No phone needed and no SMS cost. |
| `guide` | Local guide | **Bangladeshi mobile OTP** (3 steps), then profile + **NID/selfie KYC**, which an admin approves |
| `admin` | Platform staff | Created from the `ADMIN_EMAIL` / `ADMIN_PASSWORD` env vars |

## Response envelope (every endpoint)
```json
// success
{ "success": true,  "message": "logged in", "data": { ... }, "request_id": "..." }
// error
{ "success": false, "message": "human readable", "error": { "code": "OTP_INVALID", "details": { "remaining_attempts": 3 }, "retry_after_seconds": 60 }, "request_id": "..." }
```
* Base your UI logic on **`error.code`**, because it never changes. Show `message` to users.
* `VALIDATION_ERROR` (422) → `error.details` is `[{ "field": "password", "rule": "strong_password", "message": "..." }]`. Show each message next to its form field.
* On **429**, read `retry_after_seconds` (also the `Retry-After` header) and show a countdown.
* Send `request_id` with bug reports.

## Tourist sign-up flow
1. `POST /auth/register/user/send-otp` `{email}` → show the OTP screen with a resend timer (`resend_after_seconds`)
2. `POST /auth/register/user/verify-otp` `{email, otp}` → keep `data.token` in memory
3. `POST /auth/register/user/complete` `{registration_token, full_name, password?, country_code?}` → **logged in**
4. If `user.onboarding.next_step == "complete_profile"` → `PATCH /users/me/profile` (`country_code` is required)

**Google / Apple:** `POST /auth/oauth/google` or `/auth/oauth/apple` with `{id_token}` → **logged in** (`is_new_user` tells you whether to show the welcome or onboarding screens).

## Guide sign-up flow
1. `POST /auth/register/guide/send-otp` `{phone: "01712345678"}`
2. `POST /auth/register/guide/verify-otp` `{phone, otp}` → `data.token`
3. `POST /auth/register/guide/complete` `{registration_token, full_name, password}` → **logged in**
4. `PATCH /users/me/profile` (`date_of_birth`, `city`, ...) and `PUT /users/me/avatar`
5. `PUT /guides/me/profile` (headline, about, languages, service areas, rate)
6. `POST /guides/me/verification` (multipart: `nid_number`, `nid_front`, `nid_back`, `selfie`)
7. Admin approves → `verification_status = approved` → the guide appears in `GET /guides`

Use **`user.onboarding.next_step`** to route screens:
`complete_profile` → `submit_verification` → `awaiting_review` → (`resubmit_verification`) → `done`.
`onboarding.missing_fields` lists exactly which fields are missing.

## Tokens
* `access_token` is a JWT that expires in `expires_in_seconds` (default 15 min). Send it as `Authorization: Bearer <token>`.
* `refresh_token` is opaque and lasts 30 days. **It rotates on every refresh**, so always save the new one.
* Web clients should keep the refresh token in memory or secure storage, never in `localStorage` if you can avoid it. Mobile clients should use Keychain or Keystore.
* 401 `TOKEN_INVALID` → call `POST /auth/token/refresh` once, then retry. 401 `REFRESH_TOKEN_INVALID` or `SESSION_REVOKED` → go to login.
* 409 `REFRESH_TOKEN_ROTATED` → another tab already refreshed. Re-read the stored token and retry.

## Security (what the frontend will notice)
* **Rate limits** are enforced per IP (global, auth, OTP send) and per user (uploads). A request over the limit gets 429 `RATE_LIMITED`.
* **OTP:** each code is valid for 5 minutes and can be used once. You can request a new code after 60 seconds, up to 5 per hour per email or phone. After 5 wrong attempts that email or phone is blocked for `USER_BLOCK_TIME`.
* **Login lockout:** after `USER_MAX_FAILED_ATTEMPTS` wrong passwords, the account gets `ACCOUNT_TEMPORARILY_LOCKED` for `USER_BLOCK_TIME`.
* **Captcha:** when enabled, send a Cloudflare Turnstile token in the `X-Captcha-Token` header on send-otp, login and forgot-password requests.
* Forgot-password and OTP-login return the same response whether or not the account exists.
