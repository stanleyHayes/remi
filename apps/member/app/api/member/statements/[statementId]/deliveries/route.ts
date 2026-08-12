import { memberCHMS, memberJSON } from "@/lib/member-chms-proxy";

export async function POST(request: Request, context: RouteContext<"/api/member/statements/[statementId]/deliveries">) {
  const { statementId } = await context.params;
  return memberJSON(await memberCHMS(`/finance/statements/${encodeURIComponent(statementId)}/deliveries`, {
    method: "POST",
    headers: { "Idempotency-Key": request.headers.get("Idempotency-Key") || crypto.randomUUID() },
  }));
}
