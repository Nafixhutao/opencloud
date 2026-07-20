import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  // Self-contained server bundle for the Docker image (root Dockerfile);
  // `node server.js` serves the app without node_modules at runtime.
  output: 'standalone',
  experimental: {
    optimizePackageImports: ['lucide-react'],
  },
};

export default nextConfig;
