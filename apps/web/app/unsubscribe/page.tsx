import type { Metadata } from "next";
import UnsubscribeClient from "./unsubscribe-client";
import { apiBaseUrl } from "@/lib/env";

export const metadata: Metadata = { title: "Communication preferences", robots:{index:false,follow:false} };

export default async function UnsubscribePage({searchParams}:{searchParams:Promise<{token?:string}>}) {
  const {token=""}=await searchParams;
  return <main className="unsubscribe-page"><div className="unsubscribe-orbit" aria-hidden="true"><i/><i/></div><section><span className="unsubscribe-mark">R</span><p>COMMUNICATION CARE</p><h1>Your inbox.<br/><em>Your choice.</em></h1><p className="unsubscribe-copy">Stop this type of message on this channel. Essential account and safety notices are managed separately.</p><UnsubscribeClient apiBase={apiBaseUrl} token={token}/><a href="/">Return to REMI</a></section></main>
}
