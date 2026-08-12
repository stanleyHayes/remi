import { api } from "./api";

export type MemberSection = "identity" | "household" | "journey" | "attendance" | "groups" | "serving" | "care" | "giving";

export interface MemberContact {
  type: string;
  value: string;
  primary?: boolean;
  verifiedAt?: string;
}

export interface MemberProfile {
  person: {
    id: string;
    version: number;
    personNumber: string;
    names: { given: string; middle?: string; family?: string; preferred?: string };
    photoAssetId?: string;
    photoUrl?: string;
    homeBranchId: string;
    homeBranchName?: string;
    membershipStage: string;
    aliases?: string[];
    dateOfBirth?: { value: string; precision: "year" | "month" | "day" };
    gender?: string;
    contactPoints?: MemberContact[];
    addresses?: Array<{ label?: string; line1: string; line2?: string; city?: string; region?: string; country: string; primary?: boolean }>;
    tags?: string[];
    customFields?: Record<string, string>;
    communicationPreferences?: { email?: boolean; sms?: boolean; whatsapp?: boolean; phone?: boolean };
    createdAt?: string;
    updatedAt?: string;
    archivedAt?: string;
  };
  permissions?: { sections?: MemberSection[]; actions?: string[] };
  household?: {
    id: string;
    name: string;
    role?: string;
    primaryContactId?: string;
    members: Array<{ id: string; name: string; role?: string; relationship?: string; photoUrl?: string }>;
    sharedAddress?: string;
  };
  journey?: Array<{ id: string; fromStage: string; toStage: string; effectiveAt: string; reasonCode: string; reversed?: boolean }>;
  attendance?: {
    lastSeenAt?: string;
    services90Days: number;
    currentStreak?: number;
    records: Array<{ id: string; occurredAt: string; serviceName: string; branchName?: string; source?: string }>;
  };
  groups?: Array<{ id: string; name: string; type?: string; role?: string; joinedAt?: string; status?: string }>;
  serving?: Array<{ id: string; team: string; position?: string; status?: string; nextServingAt?: string }>;
  care?: { openCases: number; overdueTasks: number; lastContactAt?: string; cases?: Array<{ id: string; category: string; status: string; urgency?: string; assignedTo?: string; updatedAt?: string }> };
  giving?: { currency: string; yearToDateMinor: number; lastGiftAt?: string; fundsSupported?: number; statementAvailable?: boolean };
}

export interface PeoplePage {
  items: Array<{
    id: string;
    version: number;
    personNumber: string;
    names: MemberProfile["person"]["names"];
    photoUrl?: string;
    homeBranchId: string;
    homeBranchName?: string;
    membershipStage: string;
    tags?: string[];
    updatedAt?: string;
  }>;
  nextCursor?: string;
  hasMore?: boolean;
}

export function displayMemberName(names: MemberProfile["person"]["names"]) {
  return names.preferred || [names.given, names.middle, names.family].filter(Boolean).join(" ");
}

export function loadMemberProfile(id: string) {
  return api<MemberProfile>(`/api/chms/v1/people/${encodeURIComponent(id)}?include=household,journey,attendance,groups,serving,care,giving`);
}

export function loadPeople(query = "", branchId = "") {
  const search = new URLSearchParams({ limit: "50" });
  if (query.trim()) search.set("q", query.trim());
  if (branchId) search.set("branchId", branchId);
  return api<PeoplePage>(`/api/chms/v1/people?${search.toString()}`);
}

const ROLE_SECTIONS: Record<string, MemberSection[]> = {
  "super-admin": ["identity", "household", "journey", "attendance", "groups", "serving", "care", "giving"],
  "membership-admin": ["identity", "household", "journey", "attendance", "groups", "serving"],
  editor: ["identity", "household", "journey", "attendance", "groups", "serving"],
  pastor: ["identity", "household", "journey", "attendance", "groups", "serving", "care"],
  "finance-admin": ["identity", "household", "giving"],
  viewer: ["identity", "household", "attendance", "groups"],
};

export function visibleMemberSections(role: string | undefined, serverSections?: MemberSection[]) {
  const clientAllowed = ROLE_SECTIONS[role || "viewer"] || ROLE_SECTIONS.viewer;
  if (!serverSections) return clientAllowed;
  return clientAllowed.filter((section) => serverSections.includes(section));
}
