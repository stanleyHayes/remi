"use client";

import { useCallback, useEffect, useState } from "react";
import { api, asList, ApiError, getStoredUser } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { SkeletonTable } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";

interface AdminUserRow {
  id: string;
  email: string;
  name?: string;
  role?: string;
  invitationStatus?: "pending" | "accepted";
  invitedAt?: string;
  createdAt?: string;
	branchIds?: string[];
  ministryIds?: string[];
	accessVersion?: number;
}
interface ScopeOption { id: string; name: string }

interface InvitationResult {
  demoInvitationUrl?: string;
}

const ROLES = [
  { value: "viewer", label: "Viewer", note: "Read-only general workspace" },
  { value: "editor", label: "Content editor", note: "Website publishing and community inbox" },
  { value: "pastor", label: "Pastor", note: "Pastoral care, attendance and connection" },
  { value: "branch-admin", label: "Branch administrator", note: "Local church operations" },
  { value: "membership-admin", label: "Membership administrator", note: "People, households and attendance" },
  { value: "group-admin", label: "Groups administrator", note: "Groups, rosters and ministry insights" },
  { value: "volunteer-coordinator", label: "Volunteer coordinator", note: "Teams, schedules and serving coverage" },
  { value: "finance-counter", label: "Finance counter", note: "Counting batches only" },
  { value: "finance-admin", label: "Finance administrator", note: "Finance configuration and ledger" },
  { value: "finance-approver", label: "Finance approver", note: "Controlled approvals and exports" },
  { value: "finance-auditor", label: "Finance auditor", note: "Read-only finance assurance" },
  { value: "auditor", label: "Operational auditor", note: "Read and export safe branch audit metadata" },
  { value: "data-protection-supervisor", label: "Data protection supervisor", note: "Organization privacy and audit assurance" },
  { value: "super-admin", label: "Super administrator", note: "Full organization access" },
];

const input =
  "mt-1.5 w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-gold";

export default function UsersPage() {
  const { showToast } = useToast();
  const [users, setUsers] = useState<AdminUserRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  const [inviteOpen, setInviteOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("editor");
	const [branches, setBranches] = useState<ScopeOption[]>([]);
	const [ministries, setMinistries] = useState<ScopeOption[]>([]);
	const [branchIds, setBranchIds] = useState<string[]>([]);
	const [ministryIds, setMinistryIds] = useState<string[]>([]);
	const [scopeUser,setScopeUser]=useState<AdminUserRow|null>(null);
	const [scopeReason,setScopeReason]=useState("");
	const [scopeRole,setScopeRole]=useState("viewer");
  const [inviting, setInviting] = useState(false);

  const load = useCallback(async () => {
    setError(null);
    setForbidden(false);
    try {
      const data = await api("/api/admin/users");
      setUsers(asList<AdminUserRow>(data));
    } catch (err) {
      if (err instanceof ApiError && (err.status === 403 || err.status === 401)) {
        setForbidden(true);
      } else {
        setError(err instanceof ApiError ? err.message : "Failed to load users.");
      }
      setUsers([]);
    }
  }, []);

  useEffect(() => {
    load();
	Promise.all([api("/api/branches"), api("/api/ministries")]).then(([branchData, ministryData]) => {
		setBranches(asList<ScopeOption>(branchData));
		setMinistries(asList<ScopeOption>(ministryData));
	}).catch(() => {});
  }, [load]);

  function resetInvite() {
    setEmail("");
    setRole("editor");
	setBranchIds([]);
	setMinistryIds([]);
  }

  async function onInvite(e: React.FormEvent) {
    e.preventDefault();
    setInviting(true);
    try {
      const result = await api<InvitationResult>("/api/admin/users", { method: "POST", body: { email, role, branchIds, ministryIds } });
      if (result.demoInvitationUrl) {
        await navigator.clipboard?.writeText(result.demoInvitationUrl);
        showToast(`Invitation created. Demo link copied to your clipboard.`);
      } else {
        showToast(`Invitation sent to ${email}.`);
      }
      setInviteOpen(false);
      resetInvite();
      load();
    } catch (err) {
      showToast(err instanceof ApiError ? err.message : "Invite failed.", "error");
    } finally {
      setInviting(false);
    }
  }

	const operationalRole = !["viewer", "editor", "super-admin"].includes(role);
	const ministryRole = ["pastor", "group-admin", "volunteer-coordinator"].includes(role);
	function toggleScope(id:string, selected:string[], update:(value:string[])=>void){ update(selected.includes(id)?selected.filter(item=>item!==id):[...selected,id]); }
	function editScope(user:AdminUserRow){setScopeUser(user);setScopeRole(user.role||"viewer");setBranchIds((user.branchIds||[]).filter(id=>branches.some(item=>item.id===id)));setMinistryIds((user.ministryIds||[]).filter(id=>ministries.some(item=>item.id===id)));setScopeReason("")}
	async function saveScope(){if(!scopeUser)return;setInviting(true);try{await api(`/api/admin/users/${encodeURIComponent(scopeUser.id)}/scopes`,{method:"PATCH",body:{role:scopeRole,branchIds,ministryIds,expectedAccessVersion:scopeUser.accessVersion||0,reason:scopeReason}});showToast("Access role and scope updated. Existing sessions were revoked.");setScopeUser(null);resetInvite();await load()}catch(err){showToast(err instanceof ApiError?err.message:"Access update failed.","error")}finally{setInviting(false)}}

  return (
    <div>
      <PageHeader title="Users" subtitle="Admin accounts — manage who can edit and publish (super-admin only)">
        {users !== null && !forbidden && (
          <button
            onClick={() => setInviteOpen(true)}
            className="rounded-md bg-gold px-4 py-2 text-sm font-semibold text-sidebar transition hover:bg-gold-dark"
          >
            + Invite user
          </button>
        )}
      </PageHeader>

      {forbidden && (
        <EmptyState
          title="Super-admin access required"
          hint="Your account does not have permission to manage users."
        />
      )}
      {error && <ErrorBox message={error} onRetry={load} />}
      {users === null && !error && !forbidden && <SkeletonTable rows={4} cols={3} />}

      {users !== null && !forbidden && (
        <div className="max-w-3xl">
          {users.length === 0 ? (
            <EmptyState title="No admin users found" />
          ) : (
            <Card className="overflow-x-auto">
              <table className="w-full min-w-[480px] text-left text-sm">
                <thead>
                  <tr className="border-b border-zinc-200 text-xs uppercase tracking-wide text-zinc-500">
                    <th className="px-4 py-3 font-medium">Name</th>
                    <th className="px-4 py-3 font-medium">Email</th>
                    <th className="px-4 py-3 font-medium">Access</th>
					<th className="px-4 py-3 font-medium">Scope</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((u) => (
                    <tr key={u.id ?? u.email} className="border-b border-zinc-100 last:border-0 hover:bg-zinc-50">
                      <td className="px-4 py-3 font-medium text-zinc-900">{u.name || <span className="text-zinc-400">Awaiting setup</span>}</td>
                      <td className="px-4 py-3 text-zinc-600">{u.email}</td>
                      <td className="px-4 py-3">
                        <StatusBadge status={u.role} />
                      </td>
					  <td className="px-4 py-3 text-zinc-600"><span className="user-scope-summary">{u.role==="super-admin"?"All branches":u.branchIds?.length?`${u.branchIds.length} branch${u.branchIds.length===1?"":"es"}`:"Content only"}{u.ministryIds?.length&&u.role!=="super-admin"?<small>{u.ministryIds.length} ministries</small>:null}{u.id!==getStoredUser()?.id&&<button type="button" onClick={()=>editScope(u)}>Edit access</button>}</span></td>
                      <td className="px-4 py-3">
                        <StatusBadge status={u.invitationStatus === "pending" ? "pending" : "active"} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          )}
        </div>
      )}

      <Modal open={inviteOpen} onClose={() => !inviting && setInviteOpen(false)} title="Invite user">
        <form onSubmit={onInvite} className="space-y-4">
          <p className="rounded-xl border border-amber-200/70 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900">
            We&apos;ll email a secure, one-time link. The recipient will enter their own name and create a password when they accept it. Invitations expire after 48 hours.
          </p>
          <label className="block text-sm font-medium text-zinc-700">
            Email
            <input required type="email" autoComplete="email" className={input} value={email} onChange={(e) => setEmail(e.target.value)} placeholder="teammate@example.com" />
          </label>
          <span className="block text-sm font-medium text-zinc-700">
            Role
            <Select className="mt-1.5" value={role} onChange={(e) => setRole(e.target.value)}>
              {ROLES.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label} — {item.note}
                </option>
              ))}
            </Select>
            <span className="mt-1 block text-xs font-normal text-zinc-400">
              Access is permission-scoped by ministry domain. Finance and pastoral data remain isolated.
            </span>
          </span>
		  {operationalRole && <fieldset className="user-scope-fieldset"><legend>Branch access <small>{branchIds.length} selected</small></legend><div>{branches.map(branch=><button type="button" key={branch.id} aria-pressed={branchIds.includes(branch.id)} className={branchIds.includes(branch.id)?"is-selected":""} onClick={()=>toggleScope(branch.id,branchIds,setBranchIds)}><i aria-hidden="true">{branchIds.includes(branch.id)?"✓":"+"}</i><span><b>{branch.name}</b><small>Operational records in this branch</small></span></button>)}</div><p>At least one branch is required. Requests without an explicit matching branch are denied.</p></fieldset>}
		  {ministryRole && ministries.length>0 && <fieldset className="user-scope-fieldset"><legend>Ministry access <small>{ministryIds.length} selected</small></legend><div>{ministries.map(ministry=><button type="button" key={ministry.id} aria-pressed={ministryIds.includes(ministry.id)} className={ministryIds.includes(ministry.id)?"is-selected":""} onClick={()=>toggleScope(ministry.id,ministryIds,setMinistryIds)}><i aria-hidden="true">{ministryIds.includes(ministry.id)?"✓":"+"}</i><span><b>{ministry.name}</b><small>Only ministry-owned groups and workflows</small></span></button>)}</div></fieldset>}

          <button
            type="submit"
            disabled={inviting || (operationalRole && branchIds.length===0)}
            className="w-full rounded-md bg-gold px-4 py-2.5 text-sm font-semibold text-sidebar transition hover:bg-gold-dark disabled:opacity-60"
          >
            {inviting ? "Sending invitation…" : "Send secure invitation"}
          </button>
        </form>
      </Modal>
	  <Modal open={Boolean(scopeUser)} onClose={()=>!inviting&&setScopeUser(null)} title={`Edit ${scopeUser?.name||scopeUser?.email||"user"} access`}>
		<div className="space-y-4"><p className="scope-change-warning">Saving immediately revokes every existing staff session for this account. Role changes additionally require your recent MFA verification.</p><label className="scope-role"><span>Staff role</span><Select value={scopeRole} onChange={event=>setScopeRole(event.target.value)}>{ROLES.map(item=><option key={item.value} value={item.value}>{item.label} — {item.note}</option>)}</Select></label>{scopeRole!=="super-admin"&&<><fieldset className="user-scope-fieldset"><legend>Branch access <small>{branchIds.length} selected</small></legend><div>{branches.map(branch=><button type="button" key={branch.id} aria-pressed={branchIds.includes(branch.id)} className={branchIds.includes(branch.id)?"is-selected":""} onClick={()=>toggleScope(branch.id,branchIds,setBranchIds)}><i aria-hidden="true">{branchIds.includes(branch.id)?"✓":"+"}</i><span><b>{branch.name}</b><small>Operational records in this branch</small></span></button>)}</div></fieldset><fieldset className="user-scope-fieldset"><legend>Ministry access <small>{ministryIds.length} selected</small></legend><div>{ministries.map(ministry=><button type="button" key={ministry.id} aria-pressed={ministryIds.includes(ministry.id)} className={ministryIds.includes(ministry.id)?"is-selected":""} onClick={()=>toggleScope(ministry.id,ministryIds,setMinistryIds)}><i aria-hidden="true">{ministryIds.includes(ministry.id)?"✓":"+"}</i><span><b>{ministry.name}</b><small>Ministry-owned groups and workflows</small></span></button>)}</div></fieldset></>}<label className="scope-reason"><span>Reason for access change</span><textarea value={scopeReason} onChange={event=>setScopeReason(event.target.value)} placeholder="Explain why this access is changing"/></label><button type="button" className="admin-primary-button scope-save-button" disabled={inviting||scopeReason.trim().length<8||Boolean(!['viewer','editor','super-admin'].includes(scopeRole)&&!branchIds.length)} onClick={()=>void saveScope()}>{inviting?"Saving…":"Save and revoke sessions"}</button></div>
	  </Modal>
    </div>
  );
}
