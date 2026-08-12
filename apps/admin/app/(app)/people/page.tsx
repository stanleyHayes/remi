"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, ApiError, asList } from "@/lib/api";
import { displayMemberName, loadPeople, type PeoplePage } from "@/lib/chms";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import { SkeletonTable } from "@/components/ui/Skeleton";

interface Branch { id: string; name: string }

export default function PeopleDirectoryPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branchId, setBranchId] = useState("");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [data, setData] = useState<PeoplePage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api("/api/branches").then((value) => {
      const items = asList<Branch>(value); setBranches(items); setBranchId((current) => current || items[0]?.id || "");
    }).catch(() => setBranches([]));
  }, []);

  const load = useCallback(async () => {
    setLoading(true); setError(null);
    try { setData(await loadPeople(submittedQuery, branchId)); }
    catch (cause) { setError(cause instanceof ApiError ? cause.message : "The people directory could not be loaded."); }
    finally { setLoading(false); }
  }, [branchId, submittedQuery]);

  useEffect(() => { load(); }, [load]);
  function submit(event: FormEvent) { event.preventDefault(); setSubmittedQuery(query.trim()); }

  return <div>
    <PageHeader title="People" subtitle="Find members, understand their ministry journey and care for every household with context.">
      <Link href="/people/import" className="admin-secondary-action rounded-lg px-4 py-2.5 text-sm font-semibold">Import CSV</Link>
      <Link href="/people/new" className="admin-primary-button rounded-lg px-4 py-2.5 text-sm font-semibold">Add person</Link>
    </PageHeader>
    <Card className="mb-5 p-4">
      <form onSubmit={submit} className="grid gap-3 md:grid-cols-[minmax(0,1fr)_15rem_auto]">
        <label className="block"><span className="sr-only">Search people</span><div className="admin-search-control flex min-h-11 items-center gap-2 rounded-lg border border-zinc-200 bg-white px-3"><span aria-hidden="true" className="text-zinc-400">⌕</span><input className="min-w-0 flex-1 !border-0 !bg-transparent !p-0 !shadow-none" value={query} onChange={(event)=>setQuery(event.target.value)} placeholder="Name, person number, email or phone" /></div></label>
        <label className="block"><span className="sr-only">Branch</span><Select aria-label="Branch" value={branchId} onChange={(event)=>setBranchId(event.target.value)}><option value="">All permitted branches</option>{branches.map(branch=><option key={branch.id} value={branch.id}>{branch.name}</option>)}</Select></label>
        <button className="admin-secondary-action min-h-11 rounded-lg px-5 text-sm font-semibold" type="submit">Search</button>
      </form>
    </Card>
    {loading && <SkeletonTable rows={7} cols={5}/>} 
    {error && <ErrorBox message={error} onRetry={load}/>} 
    {!loading&&!error&&data&&data.items.length===0&&<EmptyState title={submittedQuery?"No people matched":"Your people directory is ready"} hint={submittedQuery?"Try a name, member number or another permitted branch.":"Add the first person or import an existing member list."} action={<Link className="admin-primary-button rounded-lg px-4 py-2 text-sm font-semibold" href="/people/new">Add person</Link>}/>} 
    {!loading&&!error&&data&&data.items.length>0&&<Card className="overflow-hidden"><div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left"><thead><tr><th className="px-5 py-3 text-[10px] uppercase tracking-[.12em]">Person</th><th className="px-4 py-3 text-[10px] uppercase tracking-[.12em]">Stage</th><th className="px-4 py-3 text-[10px] uppercase tracking-[.12em]">Branch</th><th className="px-4 py-3 text-[10px] uppercase tracking-[.12em]">Tags</th><th className="px-5 py-3 text-right text-[10px] uppercase tracking-[.12em]">Record</th></tr></thead><tbody>{data.items.map(person=><tr key={person.id}><td className="px-5 py-4"><Link href={`/people/${person.id}`} className="flex items-center gap-3"><span className="member-row-icon">{initials(person.names.given,person.names.family)}</span><span><b className="block text-sm text-[var(--remi-ink)]">{displayMemberName(person.names)}</b><small className="mt-1 block font-mono text-[10px] text-[var(--remi-muted)]">{person.personNumber}</small></span></Link></td><td className="px-4 py-4"><span className="member-tag">{label(person.membershipStage)}</span></td><td className="px-4 py-4 text-xs text-[var(--remi-muted)]">{person.homeBranchName||person.homeBranchId}</td><td className="px-4 py-4"><div className="flex max-w-60 flex-wrap gap-1.5">{person.tags?.slice(0,3).map(tag=><span key={tag} className="member-profile-muted-badge !text-[10px]">{tag}</span>)}{(person.tags?.length||0)>3&&<span className="text-[10px] text-[var(--remi-muted)]">+{(person.tags?.length||0)-3}</span>}</div></td><td className="px-5 py-4 text-right"><Link href={`/people/${person.id}`} className="admin-secondary-action inline-flex rounded-lg px-3 py-2 text-xs font-semibold">View</Link></td></tr>)}</tbody></table></div></Card>}
  </div>;
}

function initials(first:string,last?:string){return `${first.trim().charAt(0)}${(last||"").trim().charAt(0)}`.toUpperCase()||"M";}
function label(value:string){return value.replaceAll("-"," ").replace(/\b\w/g,(letter)=>letter.toUpperCase());}
