import { NextRequest, NextResponse } from "next/server";
import { memberJSON } from "@/lib/member-chms-proxy";
import { cookies } from "next/headers";
import { apiURL } from "@/lib/server-api";

function target(path: string, method: string) {
  if (method === "GET" && path === "workspace") return "/api/member/care-content";
  if (method === "POST" && path === "prayer") return "/api/member/prayer-requests";
  if (method === "POST" && path === "request") return "/api/member/care-requests";
  if (method === "POST" && path === "appointment") return "/api/member/pastoral-appointments";
  if (method === "PUT" && /^save\/sermon\/[A-Fa-f0-9]{24}$/.test(path)) return `/api/member/saved-content/${path.slice(5)}`;
  return "";
}

async function forward(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const { path: parts } = await context.params; const path = parts.join("/"); const endpoint = target(path, request.method);
  if (!endpoint) return NextResponse.json({ error: "Care operation is not available." }, { status: 404 });
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const body = request.method === "GET" ? undefined : await request.text();
  const response = await fetch(apiURL(endpoint), { method: request.method, body: body || undefined, headers: { Authorization: `Bearer ${access}`, ...(body ? { "Content-Type": "application/json" } : {}) }, cache: "no-store" });
  return memberJSON(response);
}
export const GET = forward; export const POST = forward; export const PUT = forward;
