import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { memberJSON } from "@/lib/member-chms-proxy";
import { apiURL } from "@/lib/server-api";

function endpoint(path: string, method: string, query: string) {
  if (method === "GET" && path === "workspace") return "/api/member/communication-workspace";
  if (method === "PUT" && path === "preferences") return "/api/member/communication-preferences";
  if (method === "GET" && path === "directory") return `/api/member/directory${query}`;
  if (method === "PATCH" && /^inbox\/[A-Fa-f0-9]{24}$/.test(path)) return `/api/member/${path}`;
  if (["GET", "POST"].includes(method) && /^groups\/[A-Za-z0-9_-]{1,64}\/messages$/.test(path)) return `/api/member/${path}`;
  return "";
}

async function forward(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const path = (await context.params).path.join("/");
  const target = endpoint(path, request.method, request.nextUrl.search);
  if (!target) return NextResponse.json({ error: "Message operation is not available." }, { status: 404 });
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const body = request.method === "GET" ? undefined : await request.text();
  const response = await fetch(apiURL(target), { method: request.method, body: body || undefined, headers: { Authorization: `Bearer ${access}`, ...(body ? { "Content-Type": "application/json" } : {}) }, cache: "no-store" });
  return memberJSON(response);
}

export const GET = forward;
export const POST = forward;
export const PUT = forward;
export const PATCH = forward;
