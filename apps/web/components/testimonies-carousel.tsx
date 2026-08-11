"use client";

import { useEffect, useState } from "react";
import type { Testimony } from "@/lib/types";

export default function TestimoniesCarousel({ testimonies }: { testimonies: Testimony[] }) {
  const [index, setIndex] = useState(0);

  useEffect(() => {
    if (testimonies.length <= 1) return;
    const t = setInterval(() => setIndex((i) => (i + 1) % testimonies.length), 7000);
    return () => clearInterval(t);
  }, [testimonies.length]);

  if (testimonies.length === 0) return null;
  const current = testimonies[index];

  return (
    <div className="mx-auto max-w-3xl text-center">
      <div key={current.id} className="reveal is-visible">
        <svg className="mx-auto mb-8 text-gold/50" width="36" height="36" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
          <path d="M10 8c-3 1-5 3.5-5 7v1h5v-6H7.5C8 9 9 8.5 10 8.2V8Zm9 0c-3 1-5 3.5-5 7v1h5v-6h-2.5C17 9 18 8.5 19 8.2V8Z" />
        </svg>
        <blockquote className="font-display text-2xl font-medium leading-relaxed text-cream sm:text-3xl">
          {current.body}
        </blockquote>
        <p className="mt-8 text-[11px] font-medium uppercase tracking-[0.25em] text-gold-bright">
          {current.author}
        </p>
      </div>

      {testimonies.length > 1 ? (
        <div className="mt-10 flex justify-center gap-2.5">
          {testimonies.map((t, i) => (
            <button
              key={t.id}
              type="button"
              onClick={() => setIndex(i)}
              aria-label={`Show testimony ${i + 1}`}
              className={`h-1.5 rounded-full transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] ${
                i === index ? "w-8 bg-gold" : "w-1.5 bg-cream/20 hover:bg-cream/40"
              }`}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}
