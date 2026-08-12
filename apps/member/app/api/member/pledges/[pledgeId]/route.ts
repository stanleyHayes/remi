import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

async function forward(method:"GET"|"PATCH",request:Request|undefined,context:{params:Promise<{pledgeId:string}>}){
 const access=(await cookies()).get("remi_member_access")?.value;
 if(!access)return NextResponse.json({error:"Sign in again."},{status:401});
 const{pledgeId}=await context.params;
 const response=await fetch(apiURL(`/api/member/chms/finance/pledges/${encodeURIComponent(pledgeId)}`),{method,headers:{Authorization:`Bearer ${access}`,...(request?{"Content-Type":"application/json"}:{})},body:request?await request.text():undefined,cache:"no-store"});
 return NextResponse.json(await parseResponse(response),{status:response.status});
}
export async function GET(_:Request,context:{params:Promise<{pledgeId:string}>}){return forward("GET",undefined,context)}
export async function PATCH(request:Request,context:{params:Promise<{pledgeId:string}>}){return forward("PATCH",request,context)}
