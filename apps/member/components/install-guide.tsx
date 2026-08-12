"use client";

import { useEffect, useState } from "react";

export function InstallGuide() {
  const [installed, setInstalled] = useState(false);
  const [ios, setIOS] = useState(false);
  useEffect(() => {
    setInstalled(window.matchMedia("(display-mode: standalone)").matches);
    setIOS(/iPad|iPhone|iPod/.test(navigator.userAgent));
  }, []);
  return <section className="member-install-guide"><span>INSTALL MY REMI</span><h2>{installed ? "You’re already at home." : "Keep church close at hand."}</h2><p>{installed ? "My REMI is running as an installed app on this device." : ios ? "In Safari, tap Share, then choose Add to Home Screen." : "Open your browser menu and choose Install app or Add to Home screen."}</p><div><i>01</i><b>Private by default</b><small>The app does not cache your authenticated pages or records for offline reading.</small></div><div><i>02</i><b>Always current</b><small>Reconnect before changing information, giving, registering or sending a request.</small></div></section>;
}
