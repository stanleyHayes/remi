import { memberCHMS, memberDocument } from "@/lib/member-chms-proxy";

export async function GET(_: Request, context: RouteContext<"/api/member/receipts/[receiptId]/pdf">) {
  const { receiptId } = await context.params;
  return memberDocument(await memberCHMS(`/finance/receipts/${encodeURIComponent(receiptId)}/pdf`));
}
