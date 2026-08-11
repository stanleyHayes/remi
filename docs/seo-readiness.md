# REMI SEO readiness

## Implemented

- Canonical origin: `https://remi.vercel.app`
- Unique titles, descriptions and canonical URLs for public route families
- Open Graph and Twitter cards with a generated 1200x630 REMI image
- Dynamic `/sitemap.xml` for static pages and published sermons, events and ministries
- Public `/robots.txt`; demo-success excluded
- Admin `/robots.txt` blocks all crawling, reinforced by page-level `noindex`
- `Church`, `Organization` and `WebSite` JSON-LD on the public site
- Web app manifest and consistent site identity
- Optional Google verification through `GOOGLE_SITE_VERIFICATION`

## Deployment checklist

1. Add `NEXT_PUBLIC_SITE_URL=https://remi.vercel.app` to the web Vercel project.
2. Create a Google Search Console domain or URL-prefix property.
3. Add the token portion of Google's HTML verification tag as `GOOGLE_SITE_VERIFICATION` in Vercel and redeploy.
4. Submit `https://remi.vercel.app/sitemap.xml` in Search Console.
5. Validate the homepage JSON-LD with Google's Rich Results Test.
6. Replace placeholder photography and the temporary SVG mark with approved REMI assets.
7. Add CMS fields for SEO title, description, social image and canonical override before REMI-142 is marked done.
8. Decide on analytics and consent requirements before adding tracking scripts.
9. Record mobile Lighthouse results for home, sermons, events, live and contact; target at least 90 for performance, SEO, accessibility and best practices.

## Content preparation

- Confirm the official church name, postal address, telephone and social profiles.
- Approve founder and leadership biographies before indexing them broadly.
- Build location-focused copy around Accra and each genuine branch without duplicating text.
- Give every sermon and event a descriptive title, concise summary and approved social image.
- Avoid fabricated reviews, ratings, service locations or opening hours in structured data.
