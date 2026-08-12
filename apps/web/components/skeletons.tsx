const shimmer = "animate-pulse bg-cream/[.09] motion-reduce:animate-none";

function Block({ className = "" }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`block rounded-full ${shimmer} ${className}`}
    />
  );
}

function LoadingFrame({
  children,
  label = "Loading page content",
}: {
  children: React.ReactNode;
  label?: string;
}) {
  return (
    <div role="status" aria-label={label} aria-live="polite" aria-busy="true">
      {children}
      <span className="sr-only">{label}</span>
    </div>
  );
}

export function HeaderSkeleton() {
  return (
    <header className="fixed inset-x-0 top-0 z-50 border-b border-cream/10 bg-ink/90 backdrop-blur-xl">
      <div className="mx-auto flex h-20 max-w-7xl items-center justify-between px-5 sm:px-8">
        <div className="flex items-center gap-3">
          <Block className="size-10 !rounded-xl bg-gold/20" />
          <div className="space-y-2">
            <Block className="h-3 w-20" />
            <Block className="h-2 w-14" />
          </div>
        </div>
        <div className="hidden gap-7 lg:flex">
          {Array.from({ length: 6 }, (_, i) => (
            <Block key={i} className="h-2.5 w-16" />
          ))}
        </div>
        <Block className="h-10 w-10 lg:w-24" />
      </div>
    </header>
  );
}

export function FooterSkeleton() {
  return (
    <footer className="border-t border-cream/[.07] bg-[#0c0d12]">
      <div className="mx-auto w-[min(calc(100%-2.5rem),88rem)] py-20 sm:py-28">
        <div className="grid gap-10 border-b border-cream/[.08] pb-16 lg:grid-cols-[1.45fr_.55fr]">
          <div className="space-y-5">
            <Block className="h-2.5 w-44 bg-gold/20" />
            <Block className="h-16 max-w-3xl !rounded-2xl sm:h-28" />
          </div>
          <div className="space-y-4 self-end">
            <Block className="h-3 w-full" />
            <Block className="h-3 w-4/5" />
            <Block className="h-12 w-44 bg-gold/20" />
          </div>
        </div>
        <div className="grid border-b border-cream/[.08] lg:grid-cols-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div
              key={i}
              className="space-y-4 border-b border-cream/[.08] py-10 lg:border-b-0 lg:px-10 first:pl-0"
            >
              <Block className="h-2.5 w-24 bg-gold/20" />
              <Block className="h-8 w-3/4" />
              <Block className="h-3 w-full" />
              <Block className="h-3 w-2/3" />
            </div>
          ))}
        </div>
        <div className="grid gap-10 py-12 sm:grid-cols-2">
          {Array.from({ length: 2 }, (_, i) => (
            <div key={i} className="space-y-3">
              {Array.from({ length: 5 }, (_, j) => (
                <Block key={j} className="h-10 w-full !rounded-lg" />
              ))}
            </div>
          ))}
        </div>
      </div>
    </footer>
  );
}

function HeadingSkeleton({ wide = false }: { wide?: boolean }) {
  return (
    <div className="space-y-5">
      <Block className="h-2.5 w-28 bg-gold/20" />
      <Block
        className={`h-12 !rounded-2xl sm:h-16 ${wide ? "max-w-4xl" : "max-w-2xl"}`}
      />
      <Block className="h-4 max-w-xl" />
      <Block className="h-4 max-w-md" />
    </div>
  );
}

export function HomeSkeleton() {
  return (
    <LoadingFrame label="Loading REMI home page">
      <section className="flex min-h-[100dvh] items-end bg-ink-soft">
        <div className="mx-auto w-full max-w-7xl px-5 pb-24 pt-40 sm:px-8 sm:pb-32">
          <HeadingSkeleton wide />
          <div className="mt-10 flex gap-4">
            <Block className="h-12 w-44" />
            <Block className="h-12 w-36" />
          </div>
          <div className="mt-14 grid max-w-2xl grid-cols-1 gap-6 border-t border-cream/10 pt-7 sm:grid-cols-3">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="space-y-2">
                <Block className="h-2 w-20" />
                <Block className="h-3 w-36" />
              </div>
            ))}
          </div>
        </div>
      </section>
      <section className="mx-auto max-w-7xl px-5 py-24 sm:px-8">
        <HeadingSkeleton />
        <div className="mt-12 grid gap-5 md:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }, (_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      </section>
    </LoadingFrame>
  );
}

function CardSkeleton() {
  return (
    <div className="overflow-hidden rounded-[1.5rem] border border-cream/10 bg-ink-soft p-2">
      <Block className="aspect-[16/10] w-full !rounded-[1.1rem]" />
      <div className="space-y-3 p-5">
        <Block className="h-3 w-2/5" />
        <Block className="h-6 w-4/5" />
        <Block className="h-3 w-full" />
        <Block className="h-3 w-3/4" />
      </div>
    </div>
  );
}

export function ListingSkeleton({
  cards = 6,
  columns = 3,
}: {
  cards?: number;
  columns?: 2 | 3;
}) {
  return (
    <LoadingFrame label="Loading collection">
      <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
        <HeadingSkeleton />
        <div
          className={`mt-14 grid gap-5 sm:grid-cols-2 ${columns === 3 ? "lg:grid-cols-3" : ""}`}
        >
          {Array.from({ length: cards }, (_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      </section>
    </LoadingFrame>
  );
}

export function DetailSkeleton() {
  return (
    <LoadingFrame label="Loading details">
      <section className="mx-auto max-w-5xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
        <Block className="h-3 w-28" />
        <div className="mt-9">
          <HeadingSkeleton wide />
        </div>
        <Block className="mt-12 aspect-video w-full !rounded-[2rem]" />
        <div className="mt-10 space-y-4">
          <Block className="h-4 w-full" />
          <Block className="h-4 w-11/12" />
          <Block className="h-4 w-3/4" />
        </div>
      </section>
    </LoadingFrame>
  );
}

export function EditorialSkeleton() {
  return (
    <LoadingFrame label="Loading story">
      <section className="flex min-h-[60dvh] items-end bg-ink-soft">
        <div className="mx-auto w-full max-w-4xl px-5 pb-16 pt-44 sm:px-8">
          <HeadingSkeleton />
        </div>
      </section>
      <section className="mx-auto max-w-3xl space-y-5 px-5 py-20 sm:px-8 sm:py-28">
        {Array.from({ length: 7 }, (_, i) => (
          <Block
            key={i}
            className={`h-4 ${i % 3 === 2 ? "w-3/4" : "w-full"}`}
          />
        ))}
      </section>
    </LoadingFrame>
  );
}

export function FormSkeleton() {
  return (
    <LoadingFrame label="Loading form">
      <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
        <div className="grid grid-cols-1 gap-12 lg:grid-cols-2 lg:gap-16">
          <HeadingSkeleton />
          <div className="rounded-[2rem] border border-cream/10 bg-ink-soft p-7 sm:p-9">
            <div className="space-y-5">
              {Array.from({ length: 4 }, (_, i) => (
                <div key={i} className="space-y-2">
                  <Block className="h-3 w-24" />
                  <Block className="h-12 w-full !rounded-xl" />
                </div>
              ))}
              <Block className="h-12 w-full !rounded-xl bg-gold/20" />
            </div>
          </div>
        </div>
      </section>
    </LoadingFrame>
  );
}

export function GallerySkeleton() {
  return (
    <LoadingFrame label="Loading gallery">
      <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
        <HeadingSkeleton />
        <div className="mt-14 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 9 }, (_, i) => (
            <Block
              key={i}
              className={`${i % 3 === 0 ? "aspect-[4/5]" : "aspect-[4/3]"} w-full !rounded-[1.5rem]`}
            />
          ))}
        </div>
      </section>
    </LoadingFrame>
  );
}
