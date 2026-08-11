export default function AdminSplash({ error = false, onRetry }: { error?: boolean; onRetry?: () => void }) {
  return (
    <main className="admin-splash" role={error ? "alert" : "status"} aria-live="polite" aria-busy={!error}>
      <div className="admin-splash-grid" aria-hidden="true" />
      <div className="admin-splash-orbit" aria-hidden="true"><i /><i /><i /></div>
      <section className="admin-splash-panel">
        <div className="admin-splash-mark"><span>R</span><i aria-hidden="true" /></div>
        <p className="admin-splash-kicker">REMI / Ministry operations</p>
        <h1>{error ? "Session check interrupted." : "Preparing your workspace."}</h1>
        <p className="admin-splash-copy">{error ? "Your data is unchanged. Reconnect to continue managing the ministry." : "Verifying access and assembling the latest publishing and community activity."}</p>
        {error ? (
          <button onClick={onRetry} className="admin-splash-retry">Try again <span>→</span></button>
        ) : (
          <div className="admin-splash-progress"><span /><small>Secure session handshake</small></div>
        )}
      </section>
      <footer><span>SECURE CHANNEL</span><span>ACCRA · GH</span><span>REMI OS / 01</span></footer>
    </main>
  );
}
