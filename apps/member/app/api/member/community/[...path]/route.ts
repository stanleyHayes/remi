import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

function safe(parts:string[],method:string){const path=parts.join("/");return method==="GET"&&/^groups\/[A-Za-z0-9_-]+\/community$/.test(path)||method==="PUT"&&/^groups\/[A-Za-z0-9_-]+\/(directory-visibility|meetings\/[A-Za-z0-9_-]+\/response)$/.test(path)||method==="POST"&&/^groups\/[A-Za-z0-9_-]+\/leader-messages$/.test(path)}
async function forward(request:NextRequest,context:{params:Promise<{path:string[]}>}){const access=(await cookies()).get("remi_member_access")?.value;if(!access)return NextResponse.json({error:"Sign in again."},{status:401});const{path}=await context.params;if(!safe(path,request.method))return NextResponse.json({error:"Community operation is not available."},{status:404});const body=request.method==="GET"?undefined:await request.text();const response=await fetch(apiURL(`/api/member/${path.join("/")}`),{method:request.method,body:body||undefined,headers:{Authorization:`Bearer ${access}`,...(body?{"Content-Type":"application/json"}:{})},cache:"no-store"});return NextResponse.json(await parseResponse(response),{status:response.status,headers:{"Cache-Control":"private, no-store"}})}
export const GET=forward;export const POST=forward;export const PUT=forward;
