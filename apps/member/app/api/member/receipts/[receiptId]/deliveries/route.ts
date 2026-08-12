import { memberCHMS, memberJSON } from "@/lib/member-chms-proxy";

export async function POST(request: Request, context: RouteContext<"/api/member/receipts/[receiptId]/deliveries">) {
  const { receiptId } = await context.params;
  return memberJSON(await memberCHMS(`/finance/receipts/${encodeURIComponent(receiptId)}/deliveries`, {
    method: "POST",
    headers: { "Idempotency-Key": request.headers.get("Idempotency-Key") || crypto.randomUUID() },
  }));
}
