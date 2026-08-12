import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

async function access() { return (await cookies()).get("remi_member_access")?.value; }
export async function GET() {
  const token = await access(); if (!token) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const response = await fetch(apiURL("/api/member/mfa"), { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}
export async function DELETE(request: Request) {
  const token = await access(); if (!token) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const response = await fetch(apiURL("/api/member/mfa"), { method: "DELETE", headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" }, body: await request.text(), cache: "no-store" });
  if (response.status === 204) return new NextResponse(null, { status: 204 });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}
