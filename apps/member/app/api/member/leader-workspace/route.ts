import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function GET(request: NextRequest) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const query = request.nextUrl.searchParams.toString();
  const response = await fetch(apiURL(`/api/chms/v1/leader-workspace${query ? `?${query}` : ""}`), {
    headers: { Authorization: `Bearer ${access}` }, cache: "no-store",
  });
  return NextResponse.json(await parseResponse(response), { status: response.status, headers: { "Cache-Control": "private, no-store" } });
}
