import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

export async function GET(
  _: Request,
  context: { params: Promise<{ token: string }> },
) {
  const { token } = await context.params;
  const response = await fetch(
    apiURL(`/api/member-auth/invitations/${encodeURIComponent(token)}`),
    { cache: "no-store" },
  );
  return NextResponse.json(await parseResponse(response), {
    status: response.status,
  });
}
