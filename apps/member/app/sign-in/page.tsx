import { redirect } from "next/navigation";
import { cookies } from "next/headers";
import { SignInForm } from "@/components/auth-form";

export default async function SignInPage() {
  if ((await cookies()).has("remi_member_access")) redirect("/");
  return (
    <main className="member-auth">
      <section className="member-auth-story">
        <a href="https://remi.vercel.app">
          <span>R</span>
          <div>
            <strong>MY REMI</strong>
            <small>RUACH ELOHIM MINISTRIES</small>
          </div>
        </a>
        <div>
          <p>ONE HOUSE · MANY STORIES</p>
          <blockquote>
            “You are no longer strangers, but members of God’s household.”
          </blockquote>
          <cite>Ephesians 2:19</cite>
        </div>
        <footer>Belong · Grow · Serve · Go</footer>
      </section>
      <section className="member-auth-panel">
        <SignInForm />
      </section>
    </main>
  );
}
