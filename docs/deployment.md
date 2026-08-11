# REMI deployment

## Production URLs

- Public website: `https://remi.vercel.app`
- Admin workspace: `https://remi-admin.vercel.app`
- Expected Render API: `https://remi-api.onrender.com`

If Render assigns a different API hostname, update `NEXT_PUBLIC_API_URL` in both Vercel projects and redeploy them.

## Vercel

Create two projects from the same repository:

| Project | Root Directory | Production variables |
|---|---|---|
| `remi` | `apps/web` | `NEXT_PUBLIC_API_URL=https://remi-api.onrender.com` and the existing `NEXT_PUBLIC_PAYSTACK_PUBLIC_KEY` |
| `remi-admin` | `apps/admin` | `NEXT_PUBLIC_API_URL=https://remi-api.onrender.com` |

Use the Production environment scope. App-local `.env.production` files mirror the required values for local production builds, but are intentionally ignored by Git and must not be treated as the deployment secret store.

## Render

Create or sync the Blueprint from `render.yaml`. Render builds `apps/api/Dockerfile` and exposes `/health`.

Enter every `sync: false` variable when creating the Blueprint:

- `MONGODB_URI`: a non-local MongoDB Atlas or equivalent production connection string including the database name.
- `SEED_ADMIN_EMAIL` and `SEED_ADMIN_PASSWORD`.
- `CLOUDINARY_CLOUD_NAME`, `CLOUDINARY_API_KEY`, and `CLOUDINARY_API_SECRET`.
- `RESEND_API_KEY`, `NOTIFY_EMAIL`, and `PAYSTACK_SECRET_KEY`.

Render generates `JWT_SECRET`. The Blueprint fixes CORS to both Vercel origins and invitation links to the admin URL. For an existing Blueprint, add any newly introduced `sync: false` variables manually in the Render dashboard.

## Release order

1. Provision production MongoDB and allow Render to connect.
2. Sync the Render Blueprint and enter all private variables.
3. Confirm `https://remi-api.onrender.com/health` returns HTTP 200.
4. Deploy the public Vercel project with Root Directory `apps/web`.
5. Deploy the admin Vercel project with Root Directory `apps/admin`.
6. Test public content, giving initialization, admin login, invitation links, signed uploads, and CORS from both production domains.
