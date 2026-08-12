import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

const paths: Record<string, string> = { setup: "setup", confirm: "confirm", "disable-request": "disable/request" };
export async function POST(request: Request, context: { params: Promise<{ action: string }> }) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const { action } = await context.params; const path = paths[action];
  if (!path) return NextResponse.json({ error: "Not found." }, { status: 404 });
  const response = await fetch(apiURL(`/api/member/mfa/${path}`), { method: "POST", headers: { Authorization: `Bearer ${access}`, "Content-Type": "application/json" }, body: await request.text(), cache: "no-store" });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}
