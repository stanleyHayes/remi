import { memberCHMS, memberDocument } from "@/lib/member-chms-proxy";

export async function GET(_: Request, context: RouteContext<"/api/member/statements/[statementId]/pdf">) {
  const { statementId } = await context.params;
  return memberDocument(await memberCHMS(`/finance/statements/${encodeURIComponent(statementId)}/pdf`));
}
