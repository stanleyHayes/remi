"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";

/**
 * Renders children into a portal mounted directly on document.body.
 * This escapes any ancestor overflow/hidden, transform, or backdrop-filter
 * clipping contexts so that position:fixed overlays render relative to the
 * viewport as intended.
 */
export default function Portal({ children }: { children: ReactNode }) {
  const elRef = useRef<HTMLDivElement | null>(null);

  if (!elRef.current && typeof document !== "undefined") {
    elRef.current = document.createElement("div");
  }

  useEffect(() => {
    const el = elRef.current!;
    document.body.appendChild(el);
    return () => {
      document.body.removeChild(el);
    };
  }, []);

  if (!elRef.current) return null;
  return createPortal(children, elRef.current);
}
