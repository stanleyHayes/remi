import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, authCookieOptions } from "@/lib/server-api";

export async function POST() {
  const jar = await cookies();
  const refreshToken = jar.get("remi_member_refresh")?.value ?? "";
  if (refreshToken) {
    await fetch(apiURL("/api/member-auth/logout"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken }),
      cache: "no-store",
    }).catch(() => undefined);
  }
  const response = NextResponse.json({ ok: true });
  response.cookies.set("remi_member_access", "", {
    ...authCookieOptions,
    maxAge: 0,
  });
  response.cookies.set("remi_member_refresh", "", {
    ...authCookieOptions,
    maxAge: 0,
  });
  return response;
}
