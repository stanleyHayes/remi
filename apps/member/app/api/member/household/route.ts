import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, authCookieOptions, parseResponse } from "@/lib/server-api";

export async function PATCH(request: Request) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access)
    return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const response = await fetch(apiURL("/api/member/session/household"), {
    method: "PATCH",
    headers: {
      Authorization: `Bearer ${access}`,
      "Content-Type": "application/json",
    },
    body: await request.text(),
    cache: "no-store",
  });
  const data = await parseResponse(response);
  const next = NextResponse.json(data, { status: response.status });
  if (response.ok && data.accessToken)
    next.cookies.set("remi_member_access", data.accessToken, {
      ...authCookieOptions,
      maxAge: 15 * 60,
    });
  return next;
}
