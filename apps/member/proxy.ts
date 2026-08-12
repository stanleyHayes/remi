import { NextResponse, type NextRequest } from "next/server";

export async function proxy(request: NextRequest) {
  if (request.cookies.has("remi_member_access")) return NextResponse.next();
  const refreshToken = request.cookies.get("remi_member_refresh")?.value;
  if (refreshToken) {
    const api = (
      process.env.API_URL ??
      process.env.NEXT_PUBLIC_API_URL ??
      "http://localhost:8088"
    ).replace(/\/$/, "");
    try {
      const refreshed = await fetch(`${api}/api/member-auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refreshToken }),
      });
      if (refreshed.ok) {
        const data = await refreshed.json();
        const response = NextResponse.next();
        const options = {
          httpOnly: true,
          secure: process.env.NODE_ENV === "production",
          sameSite: "lax" as const,
          path: "/",
        };
        response.cookies.set("remi_member_access", data.accessToken, {
          ...options,
          maxAge: 15 * 60,
        });
        response.cookies.set("remi_member_refresh", data.refreshToken, {
          ...options,
          maxAge: 30 * 24 * 60 * 60,
        });
        return response;
      }
    } catch {
      /* Continue to a fresh sign-in. */
    }
  }
  const response = NextResponse.redirect(new URL("/sign-in", request.url));
  response.cookies.delete("remi_member_access");
  response.cookies.delete("remi_member_refresh");
  return response;
}

export const config = { matcher: ["/", "/account/:path*", "/care/:path*", "/community/:path*", "/giving/:path*", "/lead/:path*", "/messages/:path*", "/participation/:path*", "/serving/:path*", "/support/:path*"] };
