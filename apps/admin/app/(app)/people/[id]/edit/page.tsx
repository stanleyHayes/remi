"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ApiError } from "@/lib/api";
import { displayMemberName, loadMemberProfile, type MemberProfile } from "@/lib/chms";
import { PageHeader, ErrorBox } from "@/components/ui";
import { SkeletonCard } from "@/components/ui/Skeleton";
import { PersonForm } from "@/components/people/PersonForm";

export default function EditPersonPage(){const {id}=useParams<{id:string}>();const [profile,setProfile]=useState<MemberProfile|null>(null);const [error,setError]=useState<string|null>(null);const load=useCallback(()=>{setError(null);loadMemberProfile(id).then(setProfile).catch(cause=>setError(cause instanceof ApiError?cause.message:"The member profile could not be loaded."));},[id]);useEffect(()=>{load()},[load]);if(error)return <div><Link href={`/people/${id}`} className="member-back-link">← Back to profile</Link><ErrorBox message={error} onRetry={load}/></div>;if(!profile)return <div className="space-y-5"><SkeletonCard/><SkeletonCard/><SkeletonCard/></div>;return <div><Link href={`/people/${id}`} className="member-back-link">← Back to profile</Link><PageHeader title={`Edit ${displayMemberName(profile.person.names)}`} subtitle="Changes are version-checked so another operator’s work is never silently overwritten."/><PersonForm mode="edit" person={profile.person}/></div>}
