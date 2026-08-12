import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import AccountSecurity from "@/components/account-security";
import type { HouseholdAccessResponse } from "@/components/household-access";
import type { MemberMFAStatus } from "@/components/member-mfa-settings";
import { apiURL } from "@/lib/server-api";

async function memberGet(path: string, access: string) {
  const response = await fetch(apiURL(path), {
    headers: { Authorization: `Bearer ${access}` },
    cache: "no-store",
  });
  if (!response.ok) return null;
  return response.json();
}

export default async function AccountPage() {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) redirect("/sign-in");
  const [member, sessions, households, profile, householdDetails, dataRequests, consents, householdAccess, mfa] = await Promise.all([
    memberGet("/api/member/me", access),
    memberGet("/api/member/sessions", access),
    memberGet("/api/member/households", access),
    memberGet("/api/member/profile", access),
    memberGet("/api/member/household", access),
    memberGet("/api/member/data-requests", access),
    memberGet("/api/member/consents", access),
    memberGet("/api/member/household-delegations", access),
    memberGet("/api/member/mfa", access),
  ]);
  if (!member) redirect("/sign-in");
  return (
    <AccountSecurity
      member={member}
      initialSessions={sessions ?? []}
      households={households ?? []}
      initialProfile={profile}
      initialHousehold={householdDetails}
      initialDataRequests={dataRequests ?? []}
      initialConsents={consents}
      initialHouseholdAccess={householdAccess as HouseholdAccessResponse | null}
      initialMFA={mfa as MemberMFAStatus | null}
    />
  );
}
