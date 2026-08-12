import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function POST(request: Request) {
  const body = await request.text();
  const response = await fetch(apiURL("/api/member-auth/otp/request"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
    cache: "no-store",
  });
  return NextResponse.json(await parseResponse(response), {
    status: response.status,
  });
}
