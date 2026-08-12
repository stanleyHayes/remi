"use client";

import { useEffect, useMemo, useState } from "react";
import { api, ApiError } from "@/lib/api";
import { ErrorBox, PageHeader } from "@/components/ui";
import ReportBuilder from "@/components/report-builder";
import BoardPackScheduler from "@/components/board-pack-scheduler";

type Metric = { id:string; version:string; name:string; description:string; owner:string; domain:string; unit:string; formula:string; sources:string[]; grain:string; scope:string; timezone:string; exclusions:string[]; freshnessMinutes:number; reconciliationRule:string; privacyRule:string };
type Catalog = { version:string; generatedAt:string; items:Metric[] };
const labels:Record<string,string>={attendance:"Gathering health",care:"Pastoral care",communications:"Communications","data-quality":"Data quality",finance:"Giving & finance",groups:"Groups",people:"People & households",retention:"Connection pathways",serving:"Serving"};

export default function ReportingLibraryPage(){
  const [catalog,setCatalog]=useState<Catalog|null>(null); const [loading,setLoading]=useState(true); const [error,setError]=useState<string|null>(null);
  function load(){setLoading(true);setError(null);api<Catalog>("/api/chms/v1/reporting/metrics").then(setCatalog).catch((cause)=>setError(cause instanceof ApiError?cause.message:"The reporting dictionary could not be loaded.")).finally(()=>setLoading(false));}
  useEffect(load,[]);
  const groups=useMemo(()=>{const grouped=new Map<string,Metric[]>();for(const metric of catalog?.items||[])grouped.set(metric.domain,[...(grouped.get(metric.domain)||[]),metric]);return [...grouped.entries()].sort(([a],[b])=>a.localeCompare(b));},[catalog]);
  return <div className="metric-library">
    <PageHeader title="One language for every number" subtitle="The governed source of truth behind REMI dashboards. This view follows the same role and data-sensitivity rules as the underlying ministry records." />
    <ReportBuilder />
    <BoardPackScheduler />
    <section className="metric-library-intro" aria-label="Dictionary status"><div><span>Dictionary</span><strong>{catalog?.version||"reporting-dictionary-v1"}</strong></div><div><span>Visible measures</span><strong>{loading?"··":String(catalog?.items.length||0).padStart(2,"0")}</strong></div><p>Every measure has an accountable owner, a stable formula and an explicit route back to its source records. Access is intentionally role-scoped.</p></section>
    {error&&<ErrorBox message={error} onRetry={load}/>} 
    {loading?<div className="metric-library-skeleton" aria-label="Loading metric definitions">{[0,1,2,3].map(item=><span key={item}/>)}</div>:groups.length===0?<section className="metric-library-empty"><span>Restricted view</span><h2>No reporting definitions are available to this role.</h2><p>Ask an administrator for access to the operational area you manage.</p></section>:groups.map(([domain,metrics],groupIndex)=><section className="metric-domain" key={domain}>
      <header><span>{String(groupIndex+1).padStart(2,"0")}</span><div><p>{domain.replaceAll("-"," ")}</p><h2>{labels[domain]||domain}</h2></div><strong>{metrics.length} {metrics.length===1?"measure":"measures"}</strong></header>
      <div className="metric-domain-grid">{metrics.map(metric=><article className="metric-definition" key={metric.id}>
        <div className="metric-definition-head"><div><span>{metric.id}</span><h3>{metric.name}</h3></div><b>{metric.unit}</b></div><p>{metric.description}</p>
        <dl><div><dt>Owner</dt><dd>{metric.owner}</dd></div><div><dt>Freshness</dt><dd>Within {metric.freshnessMinutes} min</dd></div><div><dt>Grain</dt><dd>{metric.grain}</dd></div><div><dt>Timezone</dt><dd>{metric.timezone}</dd></div></dl>
        <section><span>Formula</span><code>{metric.formula}</code></section><details><summary>Definition &amp; safeguards <i aria-hidden="true">+</i></summary><div className="metric-definition-detail"><p><b>Scope</b>{metric.scope}</p><p><b>Sources</b>{metric.sources.join(" · ")}</p><p><b>Excludes</b>{metric.exclusions.join("; ")}</p><p><b>Reconciles</b>{metric.reconciliationRule}</p><p><b>Privacy</b>{metric.privacyRule}</p></div></details>
      </article>)}</div>
    </section>)}
  </div>;
}
