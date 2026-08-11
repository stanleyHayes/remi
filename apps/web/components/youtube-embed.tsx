import { toEmbedUrl } from "@/lib/format";

export default function VideoEmbed({
  url,
  title,
  seed,
}: {
  url?: string | null;
  title: string;
  seed: string;
}) {
  const embed = toEmbedUrl(url);

  if (!embed) {
    return (
      <div className="relative aspect-video w-full overflow-hidden rounded-[calc(2rem-0.375rem)]">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={`https://picsum.photos/seed/${seed}/1600/900`}
          alt={title}
          className="h-full w-full object-cover opacity-70"
        />
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="flex h-16 w-16 items-center justify-center rounded-full border border-cream/20 bg-ink/60 text-cream backdrop-blur-md">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
              <path d="M8 5.5v13l11-6.5-11-6.5Z" />
            </svg>
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="aspect-video w-full overflow-hidden rounded-[calc(2rem-0.375rem)]">
      <iframe
        src={embed}
        title={title}
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
        allowFullScreen
        className="h-full w-full"
      />
    </div>
  );
}
