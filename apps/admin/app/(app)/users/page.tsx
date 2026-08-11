"use client";

import { useCallback, useEffect, useState } from "react";
import { api, asList, ApiError } from "@/lib/api";
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
}

interface InvitationResult {
  demoInvitationUrl?: string;
}

const ROLES = ["editor", "viewer", "super-admin"];

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
  }, [load]);

  function resetInvite() {
    setEmail("");
    setRole("editor");
  }

  async function onInvite(e: React.FormEvent) {
    e.preventDefault();
    setInviting(true);
    try {
      const result = await api<InvitationResult>("/api/admin/users", { method: "POST", body: { email, role } });
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
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </Select>
            <span className="mt-1 block text-xs font-normal text-zinc-400">
              viewer = read-only · editor = create & edit · super-admin = everything
            </span>
          </span>

          <button
            type="submit"
            disabled={inviting}
            className="w-full rounded-md bg-gold px-4 py-2.5 text-sm font-semibold text-sidebar transition hover:bg-gold-dark disabled:opacity-60"
          >
            {inviting ? "Sending invitation…" : "Send secure invitation"}
          </button>
        </form>
      </Modal>
    </div>
  );
}
