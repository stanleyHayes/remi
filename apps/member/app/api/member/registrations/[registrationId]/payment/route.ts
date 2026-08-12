import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function POST(_: Request, { params }: { params: Promise<{ registrationId: string }> }) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const { registrationId } = await params;
  const response = await fetch(apiURL(`/api/member/registrations/${encodeURIComponent(registrationId)}/payment`), { method: "POST", headers: { Authorization: `Bearer ${access}` }, cache: "no-store" });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}
