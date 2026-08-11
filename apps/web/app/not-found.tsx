import Link from "next/link";

export default function NotFound() {
  return (
    <section className="public-missing">
      <div className="public-missing-halo" aria-hidden="true" />
      <div className="public-missing-number" aria-hidden="true">404</div>
      <div className="public-missing-content">
        <p>Lost, but not without direction</p>
        <h1>This page isn&apos;t here.<br /><span>There is still a way forward.</span></h1>
        <p className="public-missing-copy">The link may be old, or the page may have moved. Return home or continue with the parts of REMI people visit most.</p>
        <div className="public-missing-actions">
          <Link href="/">Return home <span>→</span></Link>
          <Link href="/sermons">Browse sermons</Link>
        </div>
        <nav aria-label="Helpful destinations">
          <Link href="/events"><b>01</b> Upcoming events</Link>
          <Link href="/connect/visit"><b>02</b> Plan your visit</Link>
          <Link href="/connect/prayer"><b>03</b> Request prayer</Link>
        </nav>
      </div>
    </section>
  );
}
