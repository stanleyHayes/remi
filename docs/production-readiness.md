# Production readiness

The REMI applications build and run in production mode locally. Launch still
depends on supplying the real infrastructure and content listed below.

## Required deployment configuration

### API

- `APP_ENV=production`
- `MONGODB_URI` pointing to a backed-up, access-controlled production database
- `JWT_SECRET` generated randomly and at least 32 characters long
- `CORS_ORIGINS` containing only the deployed website and admin origins
- `SEED_ADMIN_EMAIL` and a unique `SEED_ADMIN_PASSWORD` if the seed command is
  deliberately used before launch

The API refuses to start in production with the bundled development JWT secret,
a short JWT secret, or a localhost MongoDB URI. The seed command separately
refuses production environments unless its explicit force flag is supplied.

### Frontends

- Set `NEXT_PUBLIC_API_URL` to the HTTPS API origin for both applications.
- Set `NEXT_PUBLIC_PAYSTACK_PUBLIC_KEY` on the public website when live giving
  is enabled.
- Deploy the admin application behind HTTPS. It sends no-index directives and
  security headers, but should not be linked from public navigation.

### Optional integrations required for full launch behavior

- `PAYSTACK_SECRET_KEY` for live payments
- `RESEND_API_KEY`, `EMAIL_FROM`, and `NOTIFY_EMAIL` for real notifications
- `CLOUDINARY_CLOUD_NAME`, `CLOUDINARY_API_KEY`, and
  `CLOUDINARY_API_SECRET` for uploads

Without these values, the documented demo fallbacks remain active. Any
credential previously shared in an example file must be rotated at its provider;
deleting it from the repository does not revoke it.

## Release checklist

1. Rotate exposed or demo credentials and configure deployment secrets.
2. Provision MongoDB backups, alerts, and least-privilege network access.
3. Deploy the API, then set its HTTPS origin in both frontend deployments.
4. Restrict `CORS_ORIGINS` to the final frontend origins.
5. Create a unique administrator account and remove or rotate any seeded login.
6. Replace placeholder brand assets, channel links, contact details, and seeded
   AI-draft content; approve content through the admin review queue.
7. Test login, publishing, form notifications, media upload, registration export,
   and a real low-value Paystack transaction in staging.
8. Run `pnpm --filter admin build`, `pnpm --filter admin typecheck`, and
   `cd apps/api && go test ./...` in CI before deployment.
9. Verify `/health`, backup restoration, custom domains, HTTPS, and monitoring.

## Current external blockers

- Real Cloudinary, Resend, Paystack, MongoDB, domain, and hosting credentials are
  not present in this checkout.
- Official brand assets and final client-approved content have not been supplied.
- This directory is not currently a Git checkout, so CI and a remote deployment
  cannot be proven from this workspace.
