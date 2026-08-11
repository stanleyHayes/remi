import Link from "next/link";
import { formatDate, formatTime, img } from "@/lib/format";
import type { Branch, EventItem, Leader, Ministry, Sermon } from "@/lib/types";
import { BezelCard } from "./ui";

const ease = "transition-all duration-700 ease-[cubic-bezier(0.32,0.72,0,1)]";

function CardImage({ src, alt, ratio = "aspect-[16/10]" }: { src: string; alt: string; ratio?: string }) {
  return (
    <div className={`relative ${ratio} overflow-hidden rounded-t-[calc(2rem-0.375rem)]`}>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={src}
        alt={alt}
        loading="lazy"
        className={`h-full w-full object-cover ${ease} group-hover:scale-[1.04]`}
      />
      <div className="absolute inset-0 bg-gradient-to-t from-ink/60 via-transparent to-transparent" />
    </div>
  );
}

export function SermonCard({ sermon }: { sermon: Sermon }) {
  return (
    <Link href={`/sermons/${sermon.slug}`} className="group block">
      <BezelCard>
        <CardImage src={img(sermon.image, `sermon-${sermon.slug}`)} alt={sermon.title} />
        <div className="p-6 sm:p-7">
          <div className="flex flex-wrap items-center gap-2 text-[10px] font-medium uppercase tracking-[0.18em] text-gold-bright">
            {sermon.series ? <span>{sermon.series}</span> : null}
            {sermon.series && sermon.topic ? <span className="text-cream-dim">·</span> : null}
            {sermon.topic ? <span className="text-cream-dim">{sermon.topic}</span> : null}
          </div>
          <h3 className="mt-3 font-display text-2xl font-medium leading-snug text-cream transition-colors duration-500 group-hover:text-gold-bright">
            {sermon.title}
          </h3>
          <p className="mt-3 text-sm text-cream-dim">
            {sermon.preacher}
            {sermon.date ? ` · ${formatDate(sermon.date)}` : ""}
          </p>
          {sermon.bibleRefs?.length ? (
            <p className="mt-2 text-xs italic text-cream-dim/80">{sermon.bibleRefs.join(" · ")}</p>
          ) : null}
        </div>
      </BezelCard>
    </Link>
  );
}

export function EventCard({ event }: { event: EventItem }) {
  const d = new Date(event.startAt);
  const valid = !Number.isNaN(d.getTime());
  return (
    <Link href={`/events/${event.slug}`} className="group block">
      <BezelCard>
        <div className="flex items-stretch">
          <div className="flex w-20 shrink-0 flex-col items-center justify-center rounded-l-[calc(2rem-0.375rem)] border-r border-cream/10 bg-gold/10 py-6">
            <span className="font-display text-3xl font-semibold text-gold-bright">{valid ? d.getDate() : "—"}</span>
            <span className="mt-1 text-[10px] font-medium uppercase tracking-[0.2em] text-cream-dim">
              {valid ? d.toLocaleString("en-GB", { month: "short" }) : ""}
            </span>
          </div>
          <div className="min-w-0 p-6">
            <h3 className="truncate font-display text-xl font-medium text-cream transition-colors duration-500 group-hover:text-gold-bright">
              {event.title}
            </h3>
            <p className="mt-2 text-sm text-cream-dim">
              {valid ? `${formatDate(event.startAt)} · ${formatTime(event.startAt)}` : "Date to be announced"}
            </p>
            {event.location ? <p className="mt-1 text-xs text-cream-dim/80">{event.location}</p> : null}
            {event.registrationEnabled ? (
              <span className="mt-4 inline-block rounded-full border border-gold/30 bg-gold/10 px-3 py-1 text-[10px] font-medium uppercase tracking-[0.15em] text-gold-bright">
                Registration open
              </span>
            ) : null}
          </div>
        </div>
      </BezelCard>
    </Link>
  );
}

export function MinistryCard({ ministry }: { ministry: Ministry }) {
  return (
    <Link href={`/ministries/${ministry.slug}`} className="group block h-full">
      <BezelCard className="h-full">
        <CardImage src={img(ministry.image, `ministry-${ministry.slug}`)} alt={ministry.name} ratio="aspect-[16/9]" />
        <div className="p-6 sm:p-7">
          <h3 className="font-display text-2xl font-medium text-cream transition-colors duration-500 group-hover:text-gold-bright">
            {ministry.name}
          </h3>
          <p className="mt-3 line-clamp-3 text-sm leading-relaxed text-cream-dim">{ministry.description}</p>
          <div className="mt-4 space-y-1 text-xs text-cream-dim/80">
            {ministry.leaderName ? <p>Led by {ministry.leaderName}</p> : null}
            {ministry.meetingTime ? <p>{ministry.meetingTime}</p> : null}
          </div>
        </div>
      </BezelCard>
    </Link>
  );
}

export function BranchCard({ branch }: { branch: Branch }) {
  return (
    <BezelCard className="h-full">
      <CardImage src={img(branch.image, `branch-${branch.slug}`)} alt={branch.name} ratio="aspect-[16/9]" />
      <div className="p-6 sm:p-7">
        <p className="text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">{branch.city}</p>
        <h3 className="mt-2 font-display text-2xl font-medium text-cream">{branch.name}</h3>
        <p className="mt-3 text-sm leading-relaxed text-cream-dim">{branch.address}</p>
        <div className="mt-4 space-y-1 text-xs text-cream-dim/80">
          {branch.pastorName ? <p>Pastor {branch.pastorName}</p> : null}
          {branch.serviceTimes ? <p>{branch.serviceTimes}</p> : null}
          {branch.phone ? <p>{branch.phone}</p> : null}
        </div>
        {branch.mapQuery ? (
          <a
            href={`https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(branch.mapQuery)}`}
            target="_blank"
            rel="noopener noreferrer"
            className="mt-5 inline-flex items-center gap-2 rounded-full border border-cream/15 px-4 py-2 text-xs font-medium text-cream transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] hover:border-gold/40 hover:text-gold-bright"
          >
            Open in Maps
            <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="M7 17L17 7M17 7H8M17 7v9" />
            </svg>
          </a>
        ) : null}
      </div>
    </BezelCard>
  );
}

export function LeaderCard({ leader, featured = false }: { leader: Leader; featured?: boolean }) {
  if (featured) {
    return (
      <BezelCard>
        <div className="grid md:grid-cols-2">
          <div className="relative min-h-[320px] overflow-hidden rounded-t-[calc(2rem-0.375rem)] md:rounded-l-[calc(2rem-0.375rem)] md:rounded-tr-none">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={img(leader.photo, `leader-${leader.slug}`)}
              alt={leader.name}
              className="absolute inset-0 h-full w-full object-cover"
            />
            <div className="absolute inset-0 bg-gradient-to-t from-ink/50 to-transparent md:bg-gradient-to-r" />
          </div>
          <div className="flex flex-col justify-center p-8 sm:p-12">
            <span className="inline-flex w-max items-center rounded-full border border-gold/30 bg-gold/10 px-3 py-1 text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">
              Founder &amp; General Overseer
            </span>
            <h3 className="mt-5 font-display text-3xl font-medium text-cream sm:text-4xl">{leader.name}</h3>
            <p className="mt-2 text-sm font-medium text-gold-bright">{leader.position}</p>
            <p className="mt-5 text-sm leading-relaxed text-cream-dim sm:text-base">{leader.bio}</p>
          </div>
        </div>
      </BezelCard>
    );
  }

  return (
    <BezelCard className="h-full">
      <div className="relative aspect-square overflow-hidden rounded-t-[calc(2rem-0.375rem)]">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={img(leader.photo, `leader-${leader.slug}`)}
          alt={leader.name}
          loading="lazy"
          className={`h-full w-full object-cover ${ease} hover:scale-[1.03]`}
        />
      </div>
      <div className="p-6">
        <h3 className="font-display text-xl font-medium text-cream">{leader.name}</h3>
        <p className="mt-1 text-xs font-medium uppercase tracking-[0.15em] text-gold-bright">{leader.position}</p>
        <p className="mt-3 line-clamp-3 text-sm leading-relaxed text-cream-dim">{leader.bio}</p>
      </div>
    </BezelCard>
  );
}
