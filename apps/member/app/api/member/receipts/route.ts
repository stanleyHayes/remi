import { memberCHMS, memberJSON } from "@/lib/member-chms-proxy";

export async function GET(request: Request) {
  const query = new URL(request.url).search;
  return memberJSON(await memberCHMS(`/finance/receipts${query}`));
}
