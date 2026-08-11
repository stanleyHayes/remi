import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Ruach Elohim Ministries International",
    short_name: "REMI Church",
    description: "Worship, Word, prayer, events, and community at REMI Church in Accra, Ghana.",
    start_url: "/",
    display: "standalone",
    background_color: "#101512",
    theme_color: "#172019",
    icons: [{ src: "/icon.svg", sizes: "any", type: "image/svg+xml" }],
  };
}
