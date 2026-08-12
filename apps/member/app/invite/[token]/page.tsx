import { InviteForm } from "@/components/auth-form";

export default async function InvitePage({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  return (
    <main className="member-auth">
      <section className="member-auth-story invite">
        <a href="https://remi.vercel.app">
          <span>R</span>
          <div>
            <strong>MY REMI</strong>
            <small>RUACH ELOHIM MINISTRIES</small>
          </div>
        </a>
        <div>
          <p>WELCOME TO THE FAMILY</p>
          <blockquote>
            A closer way to belong, grow, serve and stay connected through the
            week.
          </blockquote>
        </div>
        <footer>Private member access</footer>
      </section>
      <section className="member-auth-panel">
        <InviteForm token={token} />
      </section>
    </main>
  );
}
