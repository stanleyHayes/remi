"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "next/navigation";
import { api, ApiError } from "@/lib/api";
import { Card, ErrorBox } from "@/components/ui";
import { Select } from "@/components/ui/Select";

type Evidence={sourceType:string;sourceId:string;occurredAt:string;fact:string};
type Signal={id:string;version:number;ruleName:string;ruleKind:string;personId:string;observedFrom:string;observedThrough:string;evidence:Evidence[];caveats:string[];state:string;assigneeId?:string;snoozedUntil?:string;expiresAt:string;resolutionOutcome?:string;falsePositive?:boolean};
type Person={id:string;displayName:string;personNumber?:string;archived:boolean};
type Staff={id:string;name:string;email:string;role:string};
type Event={id:string;type:string;fromState:string;toState:string;assigneeId?:string;reason:string;outcome?:string;channel?:string;consentDecision?:string;occurredAt:string;actor:{id:string}};
type Detail={signal:Signal;person:Person;assignees:Staff[];events:Event[]};
type Mode="assign"|"contact"|"snooze"|"resolve"|"suppress";

const MODES:{value:Mode;label:string;copy:string}[]=[
  {value:"assign",label:"Assign",copy:"Place the observation with an authorized human reviewer."},
  {value:"contact",label:"Record outreach",copy:"Record outreach already made after a live consent re-check."},
  {value:"snooze",label:"Snooze",copy:"Hide it temporarily without erasing evidence or history."},
  {value:"resolve",label:"Resolve",copy:"Close the review with a human outcome and feedback."},
  {value:"suppress",label:"Suppress",copy:"Remove bad or contextually inappropriate evidence from the queue."},
];

export default function RetentionReviewPage(){
  const id=String(useParams<{id:string}>().id||"");
  const[data,setData]=useState<Detail|null>(null);const[mode,setMode]=useState<Mode>("assign");const[busy,setBusy]=useState(false);const[error,setError]=useState<string|null>(null);const[notice,setNotice]=useState("");
  const[assigneeId,setAssigneeId]=useState("");const[reason,setReason]=useState("");const[channel,setChannel]=useState("phone");const[contactOutcome,setContactOutcome]=useState("connected");const[resolution,setResolution]=useState("reconnected");const[feedback,setFeedback]=useState("accurate");const[suppression,setSuppression]=useState("known-context");const[snoozeDays,setSnoozeDays]=useState("7");
  const load=useCallback(async()=>{setError(null);try{const value=await api<Detail>(`/api/chms/v1/engagement/signals/${encodeURIComponent(id)}/review`);setData(value);setAssigneeId(current=>current||value.signal.assigneeId||value.assignees[0]?.id||"");setMode(value.signal.assigneeId?"contact":"assign")}catch(cause){setError(cause instanceof ApiError?cause.message:"This review could not be loaded.")}},[id]);
  useEffect(()=>{void load()},[load]);
  const selectedMode=useMemo(()=>MODES.find(item=>item.value===mode)!,[mode]);
  async function submit(event:FormEvent){event.preventDefault();if(!data)return;setBusy(true);setError(null);setNotice("");try{
    const base=`/api/chms/v1/engagement/signals/${encodeURIComponent(id)}`;let path="";let body:Record<string,unknown>={expectedVersion:data.signal.version};
    if(mode==="assign"){path="assign";body={...body,assigneeId,reason}}
    if(mode==="contact"){path="contact-events";body={...body,channel,outcome:contactOutcome,summary:reason}}
    if(mode==="snooze"){path="snooze";body={...body,until:new Date(Date.now()+Number(snoozeDays)*86400000).toISOString(),reason}}
    if(mode==="resolve"){path="resolve";body={...body,outcome:resolution,reason,falsePositive:feedback==="false-positive"}}
    if(mode==="suppress"){path="suppress";body={...body,feedbackCode:suppression,reason}}
    await api(`${base}/${path}`,{method:"POST",body});setReason("");setNotice(mode==="contact"?"Outreach record saved with a current consent decision.":"Review state updated and added to the immutable history.");await load();
  }catch(cause){setError(cause instanceof ApiError?cause.message:"The review action could not be completed.")}finally{setBusy(false)}}
  if(!data)return <div className="care-review-page"><Link className="care-back" href="/retention">← <span>Back to retention queue</span></Link>{error?<ErrorBox message={error} onRetry={load}/>:<div className="retention-skeleton"><i/><i/><i/></div>}</div>;
  const signal=data.signal;const assigned=Boolean(signal.assigneeId);const closed=signal.state==="resolved"||signal.state==="suppressed";
  return <div className="care-review-page">
    <Link className="care-back" href="/retention">← <span>Back to retention queue</span></Link>
    <header className="care-review-hero"><div><span>HUMAN REVIEW · {signal.ruleKind.replaceAll("-"," ")}</span><h1>{data.person.displayName}</h1><p>{signal.ruleName}. This is participation evidence, not a diagnosis or measure of belief.</p></div><div className={`care-state ${signal.state}`}><small>CURRENT STATE</small><b>{signal.state.replace("-"," ")}</b><em>v{signal.version}</em></div></header>
    {error&&<ErrorBox message={error} onRetry={load}/>} {notice&&<div className="retention-notice" role="status">{notice}</div>}
    <section className="care-review-grid">
      <div className="care-review-main">
        <Card className="care-evidence-card"><header><span>01 · WHY THIS APPEARED</span><h2>Observable evidence</h2></header><div className="care-evidence-window"><p><small>Observed from</small><b>{date(signal.observedFrom)}</b></p><i>→</i><p><small>Through</small><b>{date(signal.observedThrough)}</b></p><p><small>Expires</small><b>{date(signal.expiresAt)}</b></p></div>{signal.evidence.map((item,index)=><article key={item.sourceId}><b>{String(index+1).padStart(2,"0")}</b><div><h3>{item.fact}</h3><p>{item.sourceType} · {date(item.occurredAt)}</p><code>{item.sourceId}</code></div></article>)}<aside>{signal.caveats.map(item=><p key={item}>◇ {item}</p>)}</aside></Card>
        <Card className="care-history"><header><span>02 · REVIEW HISTORY</span><h2>Human decisions</h2></header>{data.events.length?<ol>{[...data.events].reverse().map(item=><li key={item.id}><i/><div><strong>{item.type.replaceAll("-"," ")}</strong><p>{item.reason}</p><small>{dateTime(item.occurredAt)} · {item.fromState} → {item.toState}{item.consentDecision?` · consent ${item.consentDecision}`:""}</small></div></li>)}</ol>:<p className="care-empty-copy">No human action has been recorded. The observation remains evidence only.</p>}</Card>
      </div>
      <aside className="care-review-rail">
        <Card className="care-person-card"><span>PERSON CONTEXT</span><h2>{data.person.displayName}</h2><p>{data.person.personNumber||"No person number"}{data.person.archived?" · Archived person":" · Active person"}</p>{data.person.archived&&<p className="care-person-warning">Outreach is blocked because this person is inactive.</p>}<Link href={`/people/${encodeURIComponent(data.person.id)}`}>View permitted profile ↗</Link></Card>
        {!closed&&<Card className="care-action-card"><header><span>REVIEW ACTION</span><h2>{selectedMode.label}</h2><p>{selectedMode.copy}</p></header><nav aria-label="Review actions">{MODES.map(item=><button key={item.value} type="button" aria-pressed={mode===item.value} disabled={(!assigned&&item.value!=="assign")||(data.person.archived&&item.value==="contact")} onClick={()=>{setMode(item.value);setReason("")}}>{item.label}</button>)}</nav><form onSubmit={submit}>
          {mode==="assign"&&<label><span>Authorized reviewer</span><Select value={assigneeId} onChange={event=>setAssigneeId(event.target.value)} required><option value="">Choose reviewer</option>{data.assignees.map(item=><option key={item.id} value={item.id}>{item.name||item.email} · {item.role}</option>)}</Select></label>}
          {mode==="contact"&&<><label><span>Channel consent checked</span><Select value={channel} onChange={event=>setChannel(event.target.value)}><option value="phone">Phone</option><option value="sms">SMS</option><option value="whatsapp">WhatsApp</option><option value="email">Email</option><option value="push">Push</option></Select></label><label><span>Outreach outcome</span><Select value={contactOutcome} onChange={event=>setContactOutcome(event.target.value)}><option value="connected">Connected</option><option value="no-answer">No answer</option><option value="message-left">Message left</option><option value="declined">Declined</option><option value="follow-up-requested">Follow-up requested</option></Select></label></>}
          {mode==="snooze"&&<fieldset><legend>Return to queue</legend><div className="care-presets">{["2","7","14","30"].map(item=><button type="button" aria-pressed={snoozeDays===item} onClick={()=>setSnoozeDays(item)} key={item}>{item}<small>days</small></button>)}</div></fieldset>}
          {mode==="resolve"&&<><label><span>Review outcome</span><Select value={resolution} onChange={event=>setResolution(event.target.value)}><option value="reconnected">Reconnected</option><option value="already-connected">Already connected</option><option value="care-offered">Care offered</option><option value="declined">Declined</option><option value="unreachable">Unreachable</option><option value="no-action-needed">No action needed</option><option value="referred">Referred</option><option value="other">Other</option></Select></label><label><span>Evidence feedback</span><Select value={feedback} onChange={event=>setFeedback(event.target.value)}><option value="accurate">Evidence was accurate</option><option value="false-positive">False positive</option></Select></label></>}
          {mode==="suppress"&&<label><span>Suppression feedback</span><Select value={suppression} onChange={event=>setSuppression(event.target.value)}><option value="known-context">Known context</option><option value="person-request">Person request</option><option value="false-positive">False positive</option><option value="duplicate-evidence">Duplicate evidence</option><option value="stale-data">Stale data</option><option value="other">Other</option></Select></label>}
          <label><span>{mode==="contact"?"Contact summary":"Reason and context"}</span><textarea value={reason} onChange={event=>setReason(event.target.value)} required minLength={3} maxLength={500} rows={4} placeholder={mode==="contact"?"Record what happened; do not paste sensitive pastoral notes here.":"Explain the human context behind this decision."}/></label>
          {mode==="contact"&&<p className="care-consent-note">This records an interaction; it does not send a message. The API re-checks current pastoral-care consent for the selected channel before saving.</p>}
          <button className="admin-primary-button" disabled={busy||(!assigned&&mode!=="assign")}>{busy?"Saving decision…":mode==="contact"?"Verify consent & record":"Save review decision"}</button>
        </form></Card>}
        {closed&&<Card className="care-closed-card"><span>REVIEW COMPLETE</span><h2>{signal.resolutionOutcome?.replaceAll("-"," ")||signal.state}</h2><p>{signal.falsePositive?"Recorded as a false positive for policy review.":"The observation is retained for audit but removed from the active queue."}</p></Card>}
      </aside>
    </section>
  </div>
}

function date(value:string){return new Date(value).toLocaleDateString(undefined,{day:"2-digit",month:"short",year:"numeric"})}
function dateTime(value:string){return new Date(value).toLocaleString(undefined,{day:"2-digit",month:"short",hour:"2-digit",minute:"2-digit"})}
