import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { apiURL } from "@/lib/server-api";

export async function POST(request: NextRequest) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const body = await request.text();
  if (body.length > 4096) return NextResponse.json({ error: "Event is too large." }, { status: 413 });
  const response = await fetch(apiURL("/api/member/client-events"), { method: "POST", body, headers: { Authorization: `Bearer ${access}`, "Content-Type": "application/json" }, cache: "no-store" });
  if (response.status === 204) return new NextResponse(null, { status: 204 });
  return NextResponse.json({ error: "Event was not accepted." }, { status: response.status });
}
