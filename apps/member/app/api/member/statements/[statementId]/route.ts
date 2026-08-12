import { memberCHMS, memberJSON } from "@/lib/member-chms-proxy";

export async function GET(_: Request, context: RouteContext<"/api/member/statements/[statementId]">) {
  const { statementId } = await context.params;
  return memberJSON(await memberCHMS(`/finance/statements/${encodeURIComponent(statementId)}`));
}
