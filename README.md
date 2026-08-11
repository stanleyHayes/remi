# REMI Platform

Website, Admin CMS and API for **Ruach Elohim Ministries International (REMI)** — founded by Dr. Ismaila Hans Awudu, Accra, Ghana.

## Apps

| App | Path | Port | Stack |
|---|---|---|---|
| Marketing website | `apps/web` | http://localhost:3010 | Next.js 16, Tailwind v4 |
| Admin CMS | `apps/admin` | http://localhost:3011 | Next.js 16, Tailwind v4 |
| API | `apps/api` | http://localhost:8088 | Go (chi), MongoDB |

> Ports 8080/3000/3001 were occupied by other projects on this machine — hence 8088/3010/3011.

## Quick start

```bash
docker compose up -d                 # MongoDB on :27019
cd apps/api && go run ./cmd/seed     # seed demo content + admin user (idempotent)
go run ./cmd/server                  # API on :8088
pnpm --filter web dev                # site on :3010   (new terminal)
pnpm --filter admin dev              # CMS on :3011    (new terminal)
```

**Admin login:** `admin@remi.church` / `remi-admin-2026`

## Configuration

All third-party services are optional in development — copy `.env.example` → `apps/api/.env` and add keys when ready:

| Keys | Service | Without keys |
|---|---|---|
| `PAYSTACK_SECRET_KEY`, `NEXT_PUBLIC_PAYSTACK_PUBLIC_KEY` | Online giving | Give page runs in demo mode (fake checkout redirect) |
| `RESEND_API_KEY` | Transactional email | Emails logged to API console |
| `CLOUDINARY_*` | Media uploads | Upload endpoint returns 501; image fields accept URLs |

## Content policy

All seeded copy is AI-generated placeholder flagged `contentStatus: "ai-draft"` or `"published"` for the demo. Review and approve real content in the admin **Review queue** (`/review`) before launch — see `agent_plan.md` §1.3.

## Docs

- `agent_plan.md` — full delivery plan, sprints, task statuses
- `docs/api-contract.md` — API specification
- `docs/production-readiness.md` — deployment inputs, security gates and launch checklist
