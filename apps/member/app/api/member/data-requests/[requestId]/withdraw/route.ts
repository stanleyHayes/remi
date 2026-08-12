import {cookies} from "next/headers";
import {NextResponse} from "next/server";
import {apiURL,parseResponse} from "@/lib/server-api";

export async function POST(request:Request,{params}:{params:Promise<{requestId:string}>}){const access=(await cookies()).get("remi_member_access")?.value;if(!access)return NextResponse.json({error:"Sign in again."},{status:401});const{requestId}=await params;const response=await fetch(apiURL(`/api/member/data-requests/${encodeURIComponent(requestId)}/withdraw`),{method:"POST",headers:{Authorization:`Bearer ${access}`,"Content-Type":"application/json"},body:await request.text(),cache:"no-store"});return NextResponse.json(await parseResponse(response),{status:response.status})}
