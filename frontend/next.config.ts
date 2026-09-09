import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  reactStrictMode: true,
  eslint: {
    // Lint chạy riêng ở CI (`npm run lint`), không chặn build.
    ignoreDuringBuilds: true,
  },
};

export default nextConfig;
