const backend = process.env.BACKEND_URL ?? "http://127.0.0.1:3001";
export default {
  compress: false, // keep NDJSON streaming from the Claude terminal unbuffered
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${backend}/api/:path*` }];
  },
};
