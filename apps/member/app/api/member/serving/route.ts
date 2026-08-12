import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function GET(request: Request) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access)
    return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const query = new URL(request.url).search;
  const response = await fetch(apiURL(`/api/member/chms/serving/assignments${query}`), {
    headers: { Authorization: `Bearer ${access}` },
    cache: "no-store",
  });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}
