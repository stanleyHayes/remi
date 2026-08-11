import { ImageResponse } from "next/og";

export const alt = "Ruach Elohim Ministries International — REMI Church";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default function OpenGraphImage() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        justifyContent: "space-between",
        padding: "72px 82px",
        color: "#f5f1e8",
        background: "#101512",
        backgroundImage:
          "radial-gradient(circle at 82% 12%, rgba(209,173,85,.28), transparent 38%), radial-gradient(circle at 8% 100%, rgba(120,154,112,.18), transparent 38%)",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 22 }}>
        <div
          style={{
            width: 70,
            height: 70,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            border: "1px solid rgba(209,173,85,.55)",
            borderRadius: 20,
            color: "#d1ad55",
            fontSize: 32,
            fontWeight: 800,
          }}
        >
          R
        </div>
        <div style={{ display: "flex", flexDirection: "column" }}>
          <span style={{ color: "#d1ad55", fontSize: 18, letterSpacing: 5 }}>REMI CHURCH</span>
          <span style={{ marginTop: 7, color: "rgba(245,241,232,.56)", fontSize: 17 }}>ACCRA · GHANA</span>
        </div>
      </div>
      <div style={{ display: "flex", maxWidth: 930, flexDirection: "column" }}>
        <div style={{ fontSize: 70, fontWeight: 700, lineHeight: 1.02, letterSpacing: -3 }}>
          Where the Spirit breathes life.
        </div>
        <div style={{ marginTop: 26, color: "rgba(245,241,232,.65)", fontSize: 25 }}>
          Worship · Word · Prayer · Community
        </div>
      </div>
    </div>,
    size,
  );
}
