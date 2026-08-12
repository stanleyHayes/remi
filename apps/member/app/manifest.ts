import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "My REMI",
    short_name: "My REMI",
    description: "Your private place to belong, grow, serve and give at REMI Church.",
    start_url: "/",
    scope: "/",
    display: "standalone",
    background_color: "#f4f0e7",
    theme_color: "#142018",
    orientation: "portrait-primary",
    categories: ["lifestyle", "social"],
    icons: [
      { src: "/icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" },
      { src: "/icon.svg", sizes: "any", type: "image/svg+xml", purpose: "maskable" },
    ],
  };
}
