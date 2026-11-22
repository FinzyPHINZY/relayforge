import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  // No page may be framed: blocks clickjacking of the sign-in buttons.
  async headers() {
    return [{ source: "/:path*", headers: [{ key: "Content-Security-Policy", value: "frame-ancestors 'none'" }, { key: "X-Frame-Options", value: "DENY" }] }];
  },
};

export default nextConfig;
