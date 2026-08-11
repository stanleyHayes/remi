import Link from "next/link";

const routes = [
  { href: "/", label: "Overview", note: "Return to ministry intelligence" },
  { href: "/review", label: "Review queue", note: "Continue publishing work" },
  { href: "/inbox", label: "Community inbox", note: "Open pastoral responses" },
];

export default function NotFound() {
  return (
    <main className="admin-missing">
      <div className="admin-missing-grid" aria-hidden="true" />
      <section>
        <div className="admin-missing-code"><span>4</span><i>R</i><span>4</span></div>
        <p className="admin-missing-kicker">Navigation exception / 404</p>
        <h1>This workspace path has no assignment.</h1>
        <p className="admin-missing-copy">The address may be outdated or the record may have moved. Choose a known operational route below.</p>
        <div className="admin-missing-routes">
          {routes.map((route, index) => <Link key={route.href} href={route.href}><b>0{index + 1}</b><span><strong>{route.label}</strong><small>{route.note}</small></span><i>→</i></Link>)}
        </div>
      </section>
      <aside aria-hidden="true"><span>ROUTE</span><b>UNRESOLVED</b><i /></aside>
    </main>
  );
}
