import type { NextConfig } from "next";

// Next 16 no longer runs ESLint during `next build` (the `eslint` config key
// was removed), so there is nothing to opt out of here.
const nextConfig: NextConfig = {};

export default nextConfig;
