import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";
async function forward(method:"POST"|"DELETE",context:{params:Promise<{groupId:string}>}){const access=(await cookies()).get("remi_member_access")?.value;if(!access)return NextResponse.json({error:"Sign in again."},{status:401});const{groupId}=await context.params;const response=await fetch(apiURL(`/api/member/groups/${encodeURIComponent(groupId)}/membership`),{method,headers:{Authorization:`Bearer ${access}`},cache:"no-store"});if(response.status===204)return new NextResponse(null,{status:204});return NextResponse.json(await parseResponse(response),{status:response.status});}
export async function POST(_:Request,context:{params:Promise<{groupId:string}>}){return forward("POST",context)}
export async function DELETE(_:Request,context:{params:Promise<{groupId:string}>}){return forward("DELETE",context)}
