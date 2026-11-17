const backend = process.env.BACKEND_URL ?? "http://127.0.0.1:3001";

// Production (Docker): NEXT_EXPORT=1 builds static files served by the Go backend.
// Development: the dev server proxies /api to the backend.
const exporting = process.env.NEXT_EXPORT === "1";

export default exporting
  ? { output: "export", trailingSlash: true }
  : {
      compress: false, // keep terminal/stream responses unbuffered
      async rewrites() {
        return [{ source: "/api/:path*", destination: `${backend}/api/:path*` }];
      },
    };
