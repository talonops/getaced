# GetAced API Documentation

Base URL: `https://api.getaced.io`

## Authentication

All protected routes require a JWT token in the Authorization header:
```
Authorization: Bearer <token>
```

The token is obtained through Google OAuth login (see below). It expires after 7 days.

---

## User Flow

The expected user journey is:

1. Sign in with Google
2. Pay (Creem checkout) OR use 7-day free trial
3. Link Google Drive
4. Set up Chrome (VNC session)
5. Complete onboarding
6. Configure settings (watch folder, bookmark folder, custom prompt)
7. Drop files into Drive folder -> answers auto-sync to Chrome bookmarks

---

## Endpoints

### Auth

#### `GET /v1/auth/google`
Starts Google OAuth login. **Redirect the user's browser here** (don't fetch it).

**Response:** Redirects to Google consent screen, then redirects to:
```
{FRONTEND_URL}/auth/callback?token=<jwt>
```

**Frontend should:**
1. Have a page at `/auth/callback`
2. Read `token` from query params
3. Store the token (localStorage, cookie, etc.)
4. Redirect user to dashboard or onboarding

---

#### `GET /v1/auth/google/drive`
**Auth required**

Returns a URL to initiate Google Drive OAuth. This is a separate consent from login - it requests `drive.readonly` permission.

**Response:**
```json
{
  "url": "https://accounts.google.com/o/oauth2/auth?..."
}
```

**Frontend should:**
1. Redirect the user to the returned `url`
2. After consent, the API redirects to `{FRONTEND_URL}/onboarding?step=chrome`
3. The user's onboarding step advances from `OnboardingStepDrive` to `OnboardingStepChrome`

**Errors:**
- `400` - User is not on the Drive onboarding step

---

### User

#### `GET /v1/me`
**Auth required**

Returns the current user's profile and status.

**Response:**
```json
{
  "id": 1,
  "email": "student@gmail.com",
  "onboarding_step": "OnboardingStepDrive",
  "subscription_status": "",
  "subscription_id": "",
  "has_drive_connected": false,
  "watch_folder_id": "",
  "bookmark_folder_name": "school work",
  "usage_count": 0,
  "usage_limit": 30,
  "trial_ends_at": "2026-03-22T00:00:00Z",
  "is_trial_active": true,
  "is_subscription_active": false,
  "is_active": true
}
```

**Key fields:**
- `onboarding_step` - One of: `OnboardingStepDrive`, `OnboardingStepChrome`, `complete`
- `is_active` - `true` if user has active subscription OR active trial. Use this to gate access.
- `usage_count` / `usage_limit` - Track how many of their 30 monthly uses they've consumed.

---

### Onboarding

The onboarding flow has 3 steps in order:

1. **Drive** (`OnboardingStepDrive`) - Link Google Drive
2. **Chrome** (`OnboardingStepChrome`) - Set up Chrome browser via VNC
3. **Complete** (`complete`) - Done

Check the user's current step via `GET /v1/me` → `onboarding_step`.

#### `POST /v1/onboarding/chrome`
**Auth required**

Creates a Chrome setup session with a VNC viewer. User must be on `OnboardingStepChrome` step.

**Response:**
```json
{
  "session_url": "https://api.getaced.io/v1/s/abc123def456"
}
```

**Frontend should:**
1. Open `session_url` in an iframe or new tab
2. User signs into their Google account inside Chrome via VNC
3. Session expires after 5 minutes
4. When user clicks "Done", call `POST /v1/onboarding/complete`

**Errors:**
- `400` - Onboarding already complete or wrong step

---

#### `POST /v1/onboarding/complete`
**Auth required**

Marks onboarding as complete. User must be on `OnboardingStepChrome` step (meaning they've already linked Drive and set up Chrome).

**Response:**
```json
{
  "onboarding_step": "complete"
}
```

**Errors:**
- `400` - Already complete or hasn't finished previous steps

---

### Payment

#### `POST /v1/checkout`
**Auth required**

Creates a Creem checkout session for subscription payment.

**Request body:**
```json
{
  "product_id": "<creem-product-id>",
  "success_url": "https://getaced.io/dashboard?payment=success"
}
```

**Response:**
```json
{
  "checkout_url": "https://checkout.creem.io/..."
}
```

**Frontend should:**
1. Redirect user to `checkout_url`
2. After payment, Creem redirects to `success_url`
3. Webhook automatically updates the user's subscription status

**Plan details:**
- $30/year
- 1-week free trial (starts on signup, no payment needed)
- 30 file processing uses per month

---

### Settings

#### `GET /v1/settings`
**Auth required**

Returns the user's current settings.

**Response:**
```json
{
  "watch_folder_id": "1aBcDeFgHiJkLmNoPqRsTuVwXyZ",
  "bookmark_folder_name": "school work",
  "custom_prompt": "",
  "has_drive_connected": true
}
```

---

#### `PUT /v1/settings/watch-folder`
**Auth required**

Sets which Google Drive folder to watch for new files. The folder ID comes from the Drive folder URL:
```
https://drive.google.com/drive/folders/1aBcDeFgHiJkLmNoPqRsTuVwXyZ
                                       ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                                       this is the folder_id
```

**Request body:**
```json
{
  "folder_id": "1aBcDeFgHiJkLmNoPqRsTuVwXyZ"
}
```

**Response:**
```json
{
  "watch_folder_id": "1aBcDeFgHiJkLmNoPqRsTuVwXyZ"
}
```

**Errors:**
- `400` - Drive not connected or folder doesn't exist / not accessible
- The folder must exist in the user's Drive and be a folder (not a file)

---

#### `PUT /v1/settings/bookmark-folder`
**Auth required**

Sets the name of the Chrome bookmark folder where answers appear. Default is "school work".

**Request body:**
```json
{
  "folder_name": "my answers"
}
```

**Response:**
```json
{
  "bookmark_folder_name": "my answers"
}
```

---

#### `PUT /v1/settings/prompt`
**Auth required**

Sets a custom prompt that gets appended to the base GPT system prompt. Use this for subject-specific instructions.

**Request body:**
```json
{
  "custom_prompt": "This is AP Chemistry, use proper scientific notation"
}
```

To clear the custom prompt, send an empty string:
```json
{
  "custom_prompt": ""
}
```

**Response:**
```json
{
  "custom_prompt": "This is AP Chemistry, use proper scientific notation"
}
```

---

### Usage

#### `GET /v1/usage`
**Auth required**

Returns the user's current usage stats and subscription status.

**Response:**
```json
{
  "usage_count": 5,
  "usage_limit": 30,
  "usage_reset_at": "2026-04-15T00:00:00Z",
  "subscription_status": "active",
  "trial_ends_at": "2026-03-22T00:00:00Z",
  "is_trial_active": false,
  "is_subscription_active": true,
  "is_active": true
}
```

**Key fields:**
- `usage_count` - Files processed this billing period
- `usage_limit` - Always 30
- `usage_reset_at` - When the counter resets (every 30 days)
- `is_active` - Whether the user can process files (has active sub or trial)

---

## How File Processing Works (Background)

This happens automatically on the backend - no frontend API calls needed:

1. Backend polls the user's watched Drive folder every 60 seconds
2. Picks up new `.png` or `.html` files
3. For HTML: strips tags, extracts visible text, sends to GPT
4. For PNG: sends image to GPT vision
5. GPT returns answers as JSON
6. Answers become folders in Chrome's bookmark bar under the configured folder name
   - Multiple choice: `1. A`
   - Open ended: `1. the answer text`
7. Chrome container is stopped, bookmarks written, restarted, synced
8. Usage counter increments

**Supported file types:** PNG, HTML only.

---

## Error Responses

All errors follow this format:
```json
{
  "error": "description of what went wrong"
}
```

**Common status codes:**
- `400` - Bad request (missing/invalid fields)
- `401` - Not authenticated (missing or invalid JWT)
- `402` - Payment required (no active subscription or trial)
- `404` - Not found
- `429` - Usage limit reached (30/month)

---

## Frontend Pages Needed

| Page | Purpose |
|---|---|
| `/auth/callback` | Receives `?token=<jwt>` after Google login, stores it |
| `/onboarding` | Steps through Drive link → Chrome setup → Complete |
| `/dashboard` | Shows usage stats, settings, subscription status |
| `/settings` | Configure watch folder, bookmark folder, custom prompt |

## Onboarding UI Flow

```
[Sign In with Google]
        ↓
  navigates to GET /v1/auth/google
        ↓
  redirects back to /auth/callback?token=...
        ↓
  store token, check GET /v1/me → onboarding_step
        ↓
┌─ OnboardingStepDrive ─────────────────────────┐
│  "Connect your Google Drive"                   │
│  [Link Google Drive] → GET /v1/auth/google/drive│
│  → redirects to /onboarding?step=chrome        │
└────────────────────────────────────────────────┘
        ↓
┌─ OnboardingStepChrome ─────────────────────────┐
│  "Set up your Chrome browser"                  │
│  POST /v1/onboarding/chrome → get session_url  │
│  show VNC iframe (5 min timer)                 │
│  [I'm Done] → POST /v1/onboarding/complete     │
└────────────────────────────────────────────────┘
        ↓
┌─ complete ─────────────────────────────────────┐
│  redirect to /dashboard                        │
│  prompt to set watch folder if not set         │
└────────────────────────────────────────────────┘
```
