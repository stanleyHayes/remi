"use client";
import {useState} from "react";

export default function UnsubscribeClient({apiBase,token}:{apiBase:string;token:string}){
 const[state,setState]=useState<"idle"|"busy"|"done"|"error">(token?"idle":"error");
 async function submit(){setState("busy");try{const response=await fetch(`${apiBase}/api/communications/opt-outs`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({token})});setState(response.ok?"done":"error")}catch{setState("error")}}
 if(state==="done")return <div className="unsubscribe-result" role="status"><b>Preference updated</b><span>This category and channel are now suppressed.</span></div>;
 return <div className="unsubscribe-action">{state==="error"&&<p role="alert">This link is missing, invalid or has expired.</p>}<button disabled={!token||state==="busy"} onClick={submit}>{state==="busy"?"Updating…":"Stop these messages"}<span>→</span></button><small>You can restore eligible preferences later from your member account.</small></div>
}
