export default function PublicSplash() {
  return (
    <div className="public-splash" role="status" aria-live="polite" aria-busy="true" aria-label="Opening REMI">
      <div className="public-splash-light" aria-hidden="true" />
      <div className="public-splash-rings" aria-hidden="true"><i /><i /><i /></div>
      <section>
        <div className="public-splash-wordmark"><span>R</span><strong>REMI</strong></div>
        <p>Ruach Elohim Ministries International</p>
        <h1>A place for the Spirit.<br />A people for the world.</h1>
        <div className="public-splash-loader" aria-hidden="true"><span /></div>
        <small>Preparing your experience</small>
      </section>
      <footer><span>Accra, Ghana</span><span>Spirit · Word · Mission</span></footer>
    </div>
  );
}
