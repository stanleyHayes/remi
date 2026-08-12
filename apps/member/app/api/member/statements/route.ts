import { memberCHMS, memberJSON } from "@/lib/member-chms-proxy";

export async function GET(request: Request) {
  const query = new URL(request.url).search;
  return memberJSON(await memberCHMS(`/finance/statements${query}`));
}

export async function POST(request: Request) {
  const query = new URL(request.url).search;
  return memberJSON(await memberCHMS(`/finance/statements${query}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: await request.text(),
  }));
}
