import { NextRequest, NextResponse } from "next/server";
import { memberCHMS, memberJSON } from "@/lib/member-chms-proxy";

function allowed(path: string, method: string) {
  if (method === "GET" && ["campaigns", "payment-methods", "recurring-instructions"].includes(path)) return true;
  if (method === "POST" && ["giving-intents", "recurring-instructions"].includes(path)) return true;
  if (method === "DELETE" && /^payment-methods\/[A-Za-z0-9_-]+$/.test(path)) return true;
  return method === "PATCH" && /^recurring-instructions\/[A-Za-z0-9_-]+$/.test(path);
}

async function forward(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const { path: parts } = await context.params;
  const path = parts.join("/");
  if (!allowed(path, request.method)) return NextResponse.json({ error: "Giving operation is not available." }, { status: 404 });
  const body = ["POST", "PATCH"].includes(request.method) ? await request.text() : undefined;
  const idempotencyKey = request.headers.get("Idempotency-Key");
  const response = await memberCHMS(`/finance/${path}`, {
    method: request.method,
    body: body || undefined,
    headers: {
      ...(body ? { "Content-Type": "application/json" } : {}),
      ...(idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {}),
    },
  });
  if (response?.status === 204) return new NextResponse(null, { status: 204 });
  return memberJSON(response);
}

export const GET = forward;
export const POST = forward;
export const PATCH = forward;
export const DELETE = forward;
