import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

const safe = (parts: string[], method: string) => {
  const path = parts.join("/");
  if (method === "GET" && /^leader-workspace\/groups\/[A-Za-z0-9_-]+$/.test(path)) return true;
	if (method === "GET" && /^leader-workspace\/teams\/[A-Za-z0-9_-]+$/.test(path)) return true;
	if (method === "POST" && /^assignments\/[A-Za-z0-9_-]+\/(reminders|no-show)$/.test(path)) return true;
  return /^groups\/[A-Za-z0-9_-]+\/(members(?:\/[A-Za-z0-9_-]+)?|meetings(?:\/[A-Za-z0-9_-]+\/attendance(?:\/[A-Za-z0-9_-]+)?)?|communication-handoffs)$/.test(path);
};

async function forward(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const { path: parts } = await context.params;
  if (!safe(parts, request.method)) return NextResponse.json({ error: "Leader operation is not available." }, { status: 404 });
  const query = request.nextUrl.searchParams.toString();
  const body = request.method === "GET" ? undefined : await request.text();
  const response = await fetch(apiURL(`/api/chms/v1/${parts.join("/")}${query ? `?${query}` : ""}`), { method: request.method, body: body || undefined, headers: { Authorization: `Bearer ${access}`, ...(body ? { "Content-Type": "application/json" } : {}) }, cache: "no-store" });
  return NextResponse.json(await parseResponse(response), { status: response.status, headers: { "Cache-Control": "private, no-store" } });
}

export const GET = forward;
export const POST = forward;
export const PATCH = forward;
export const PUT = forward;
export const DELETE = forward;
