import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function GET() {
  const response = await fetch(apiURL("/api/fundraising/campaigns"), { cache: "no-store" });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}
