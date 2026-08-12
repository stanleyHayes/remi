import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiURL, parseResponse } from "@/lib/server-api";

async function forward(method:string,request?:Request){const access=(await cookies()).get("remi_member_access")?.value;if(!access)return NextResponse.json({error:"Sign in again."},{status:401});const response=await fetch(apiURL("/api/member/data-requests"),{method,headers:{Authorization:`Bearer ${access}`,...(request?{"Content-Type":"application/json"}:{})},body:request?await request.text():undefined,cache:"no-store"});return NextResponse.json(await parseResponse(response),{status:response.status})}
export async function GET(){return forward("GET")}
export async function POST(request:Request){return forward("POST",request)}
