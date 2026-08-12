import { NextResponse } from "next/server";
import { apiURL, authCookieOptions, parseResponse } from "@/lib/server-api";

export async function POST(request: Request) {
  const response = await fetch(apiURL("/api/member-auth/otp/verify"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: await request.text(),
    cache: "no-store",
  });
  const data = await parseResponse(response);
  const next = NextResponse.json(response.ok ? (data.accessToken ? { member: data.member } : data) : data, {
    status: response.status,
  });
  if (response.ok && data.accessToken && data.refreshToken) {
    next.cookies.set("remi_member_access", data.accessToken, {
      ...authCookieOptions,
      maxAge: 15 * 60,
    });
    next.cookies.set("remi_member_refresh", data.refreshToken, {
      ...authCookieOptions,
      maxAge: 30 * 24 * 60 * 60,
    });
  }
  return next;
}
