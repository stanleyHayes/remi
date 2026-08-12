# REMI member experience research and operating model

Updated: 2026-08-11

## Product position

`apps/member` is the signed-in companion members use between gatherings. It is
not a staff console and it must not expose internal notes, retention scores,
other households, finance operations, safeguarding records, attendance
corrections, or unpublished content.

The benchmark products converge on the same core jobs: Planning Center Church
Center supports profile management, giving, event registrations, groups,
calendar, directory and forms; Pushpay adds sermons/live content, check-in,
volunteer scheduling, group engagement and prayer responses; Tithely reinforces
events, giving, notifications and group chat. REMI should cover those jobs while
keeping every action self-scoped and consent-aware.

Sources:

- [Planning Center: Set up Church Center](https://pcoaccounts.zendesk.com/hc/en-us/articles/360010614793-Set-up-Church-Center)
- [Planning Center: Chat in Planning Center](https://pcoaccounts.zendesk.com/hc/en-us/articles/31548501431067-Chat-in-Planning-Center)
- [Pushpay: Mobile church apps](https://pushpay.com/product/mobile-app/)
- [Tithely: Groups FAQs](https://help.tithe.ly/hc/en-us/articles/34327306244375-Groups-FAQs)

## Member operations map

| Area | Member jobs | Privacy and operational rule |
|---|---|---|
| Home | See the next gathering, personal calendar, announcements, pending serving responses and next steps | One self-scoped read model; no generalized member search |
| Identity | Passwordless sign-in, invitation redemption, device review, recovery and household switching | Generic account discovery responses; rotating sessions; verified household membership |
| Profile | Maintain name, photo, phone, address, emergency contact and communication choices | Verified login identifiers and staff-owned fields remain protected |
| Household | See family relationships, update shared details when primary, register/check in dependants | Guardian/primary-contact authorization is required for dependent actions |
| Events | Discover events, RSVP/register a household, complete forms/payments, cancel, waitlist and export calendar entries | Capacity and payment state are server authoritative; collect only event-required fields |
| Groups | Discover eligible groups, request/join/leave, see meetings, RSVP, contact leaders and participate in scoped chat | Directory fields are opt-in; minors and private/invite-only groups receive stricter visibility |
| Serving | Set interests, skills, availability and conflicts; accept/decline schedules; request substitutes; check in | Eligibility is staff-owned; members cannot see background-check details or another volunteer's schedule |
| Giving | Make one-time/recurring gifts, choose funds, manage provider-held methods, pledges, receipts, history and statements | REMI never stores raw payment credentials; household attribution is explicit; amounts stay out of engagement scoring |
| Check-in | Pre-check household members, receive a rotating pickup/security code and complete authorized pickup | Child rosters are never public; pickup authorization and incidents are restricted records |
| Care | Submit prayer/care requests, choose anonymous/private/group visibility, request an appointment and track only safe acknowledgements | Restricted pastoral notes and internal case state never enter the member response |
| Content | Watch live, browse sermons, save messages/notes, follow reading or next-step material | Only published content; saved activity is private by default |
| Directory | Find opted-in people within allowed branch/group scope and control one's own visibility | No bulk export, minors excluded, contact fields individually consented |
| Communication | Notifications inbox, push/email/SMS/WhatsApp preferences, group messages and quiet hours | Purpose-specific consent, unsubscribe/suppression, fatigue caps and auditable delivery |
| Support | Submit corrections/data requests, report a problem, view privacy/terms and get church contact help | Requests are verified, self-scoped and auditable |

## Information architecture

Primary mobile navigation stays limited to **Home, Events, Community, Serve,
Messages**. Giving, prayer, check-in, directory and account remain prominent
quick actions or live under a compact More/Account surface. This prevents the
bottom bar from becoming a catalogue while keeping weekly jobs one tap away.

The dashboard distinguishes three states: **Now** for time-sensitive responses,
**This week** for gatherings and commitments, and **Your life at REMI** for
next steps, household, generosity, care and content.

## Delivery sequence

The live foundation is identity, privacy-safe home, profile, household, privacy
preferences and device sessions. The next slices are community, serving,
events/check-in, generosity, care/content, and directory/notifications, followed
by the production gate. Each requires authorization tests, loading/error/empty
states, keyboard and 390 px QA, and slow-network behavior.
