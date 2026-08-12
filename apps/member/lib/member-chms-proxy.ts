import "server-only";

import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function memberCHMS(path: string, init: RequestInit = {}) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return null;
  return fetch(apiURL(`/api/member/chms${path}`), {
    ...init,
    headers: { Authorization: `Bearer ${access}`, ...(init.headers || {}) },
    cache: "no-store",
  });
}

export async function memberJSON(response: Response | null) {
  if (!response) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}

export function memberDocument(response: Response | null) {
  if (!response) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  if (!response.ok) return memberJSON(response);
  return new NextResponse(response.body, {
    status: response.status,
    headers: {
      "Content-Type": response.headers.get("Content-Type") || "application/pdf",
      "Content-Disposition": response.headers.get("Content-Disposition") || "attachment",
      "Cache-Control": "private, no-store",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
