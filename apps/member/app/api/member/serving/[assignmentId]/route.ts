import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function PATCH(request: Request, context: { params: Promise<{ assignmentId: string }> }) {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access)
    return NextResponse.json({ error: "Sign in again." }, { status: 401 });
  const { assignmentId } = await context.params;
  const response = await fetch(apiURL(`/api/member/chms/serving/assignments/${encodeURIComponent(assignmentId)}/response`), {
    method: "PATCH",
    headers: { Authorization: `Bearer ${access}`, "Content-Type": "application/json" },
    body: await request.text(),
    cache: "no-store",
  });
  return NextResponse.json(await parseResponse(response), { status: response.status });
}
