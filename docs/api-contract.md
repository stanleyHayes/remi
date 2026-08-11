# REMI API Contract (v1 — demo)

Base URL: `http://localhost:8080`. All JSON. Errors: `{ "error": "message" }` with appropriate status.
Admin endpoints require `Authorization: Bearer <jwt>`. Roles: `super-admin`, `editor`, `viewer` (viewer = read-only).

## Common fields on every content document
```json
{
  "id": "mongo-objectid-hex",
  "title": "string",
  "slug": "string",
  "contentStatus": "ai-draft | in-review | approved | published",
  "createdBy": "string", "updatedBy": "string",
  "createdAt": "RFC3339", "updatedAt": "RFC3339"
}
```
Public endpoints only return `published` (and `approved`) documents. Admin endpoints return all.

## Entities (fields in addition to common ones)

- **settings** (singleton, GET/PUT): `{ churchName, tagline, serviceTimes: [{name, day, time, location}], socials: {facebook, instagram, youtube, tiktok}, whatsapp, phone, email, address, livestreamUrl, isLive: bool, givingCategories: [string], nextServiceOverride: string|null }`
- **page**: `{ pageKey: "history|mission|beliefs|visit|about", subtitle, heroImage, body: "markdown/html string" }`
- **leader**: `{ name, position, bio, photo, order: int, isFounder: bool }`
- **ministry**: `{ name, description, image, leaderName, meetingTime }`
- **branch**: `{ name, address, city, phone, email, pastorName, serviceTimes: string, mapQuery: string, image }`
- **sermon**: `{ preacher, series, topic, bibleRefs: [string], date, videoUrl, audioUrl, notesUrl, image, description }`
- **event**: `{ description, startAt, endAt, location, image, registrationEnabled: bool, capacity: int, registeredCount: int }`
- **announcement**: `{ body, publishAt, expiresAt }`
- **testimony**: `{ author, body, image }`
- **prayer_request**: `{ name, email, request, isPrivate: bool, status: "new|in-prayer|closed" }` (no common content fields)
- **contact_submission**: `{ name, email, phone, subject, message, status: "new|read|archived" }`
- **visit_submission**: `{ name, email, phone, visitDate, branch, notes, status }`
- **event_registration**: `{ eventId, name, email, phone, createdAt }`
- **subscriber**: `{ email, status: "pending|confirmed", token }`
- **user** (admin only): `{ email, name, role }`

## Public endpoints
```
GET  /health
GET  /api/settings
GET  /api/pages/:pageKey
GET  /api/leadership
GET  /api/ministries            GET /api/ministries/:slug
GET  /api/branches
GET  /api/sermons?preacher=&series=&topic=&q=&page=   GET /api/sermons/:slug
     → list returns { items: [], total, page, pageSize, facets: { preachers: [], series: [], topics: [] } }
GET  /api/events?when=upcoming|past                   GET /api/events/:slug
POST /api/events/:id/register    { name, email, phone } → 201 { message }
GET  /api/announcements
GET  /api/testimonies
POST /api/forms/prayer           { name?, email?, request, isPrivate }
POST /api/forms/contact          { name, email, phone?, subject, message }
POST /api/forms/testimony        { author, body }
POST /api/forms/visit            { name, email, phone?, visitDate, branch?, notes? }
POST /api/subscribe              { email } → double opt-in (demo: auto-confirm)
POST /api/giving/initialize      { amount: number(pesewas), email, category }
     → { authorizationUrl, reference } — real Paystack when PAYSTACK_SECRET_KEY set, else demo URL
```

## Auth

### Account and sign-in security

- `POST /api/auth/login` returns the normal `{ token, user }` response when MFA
  is off. When MFA is enabled it returns `{ mfaRequired, challengeToken }`.
- `POST /api/auth/mfa/verify` exchanges a five-minute challenge plus a TOTP or
  unused recovery code for `{ token, user }`.
- `GET /api/auth/me` resolves the current user from the database rather than
  returning stale identity data embedded in the token.
- `PUT /api/account/profile` updates the signed-in operator and returns a
  refreshed session token.
- `PUT /api/account/password` requires the current password and a new password
  of at least 12 characters.
- `PUT /api/account/preferences` stores density, theme, digest, reduced-motion,
  and timezone preferences per operator.
- `POST /api/account/mfa/setup`, `POST /api/account/mfa/confirm`, and
  `DELETE /api/account/mfa` manage authenticator enrollment and removal.

MFA secrets, pending secrets, password hashes, and recovery-code hashes are
excluded from all normalized API responses and admin user listings.
```
POST /api/auth/login    { email, password } → { token, user }
GET  /api/auth/me       → { user }
GET  /api/auth/invitations/:token → { email, role, expiresAt }
POST /api/auth/invitations/:token/accept { name, password } → { token, user }
```

## Admin endpoints (Bearer token; writes require editor+, users require super-admin)
```
GET/POST        /api/admin/pages                 PUT /api/admin/pages/:id
GET/POST        /api/admin/leadership            PUT/DELETE /api/admin/leadership/:id
GET/POST        /api/admin/ministries            PUT/DELETE /api/admin/ministries/:id
GET/POST        /api/admin/branches              PUT/DELETE /api/admin/branches/:id
GET/POST        /api/admin/sermons               PUT/DELETE /api/admin/sermons/:id
GET/POST        /api/admin/events                PUT/DELETE /api/admin/events/:id
GET/POST        /api/admin/announcements         PUT/DELETE /api/admin/announcements/:id
GET             /api/admin/testimonies           PUT/DELETE /api/admin/testimonies/:id
GET/PUT         /api/admin/settings
PATCH           /api/admin/content/:type/:id/status   { contentStatus }  (type = pages|sermons|events|... )
GET             /api/admin/forms/prayer|contact|testimonies|visits    PATCH .../:id  { status }
GET             /api/admin/registrations?eventId=
GET             /api/admin/stats   → core counters plus { totalContent, publishedContent, publishingRate, engagementTotal, upcomingEvents, contentStatus, contentMix[], submissionMix[], engagementTrend[] }
GET             /api/admin/users   (super-admin; credential and token fields excluded)
POST            /api/admin/users   { email, role } (super-admin; sends a single-use 48-hour invitation)
POST            /api/admin/uploads/signature → Cloudinary signed params, or 501 { error } when unconfigured
```

## Fallback behaviour (no keys configured)
- No `RESEND_API_KEY` → emails logged to API stdout, requests still succeed.
- No Cloudinary keys → `uploads/signature` returns 501; frontends use placeholder image URLs.
- No `PAYSTACK_SECRET_KEY` → `giving/initialize` returns `{ authorizationUrl: "/give/demo-success?ref=...", reference, demo: true }`.
