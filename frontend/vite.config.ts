import { defineConfig, loadEnv, type ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig(({ command, mode, isPreview }) => {
  const frontendEnv = loadEnv(mode, __dirname, "VITE_");
  const backendEnv = loadEnv(mode, path.resolve(__dirname, ".."), [
    "AUTH_SERVICE_PORT",
    "USER_SERVICE_PORT",
    "PARTNERSHIP_SERVICE_PORT",
    "DOCUMENT_SERVICE_PORT",
  ]);
  const useDevProxy = command === "serve" && !isPreview &&
    frontendEnv.VITE_USE_DEV_PROXY === "true";

  const targets = {
    auth: `http://127.0.0.1:${backendEnv.AUTH_SERVICE_PORT || "8080"}`,
    user: `http://127.0.0.1:${backendEnv.USER_SERVICE_PORT || "8081"}`,
    partnership: `http://127.0.0.1:${backendEnv.PARTNERSHIP_SERVICE_PORT || "8082"}`,
    document: `http://127.0.0.1:${backendEnv.DOCUMENT_SERVICE_PORT || "8083"}`,
  };
  const apiServices = {
    VITE_AUTH_API_BASE_URL: "auth",
    VITE_ADMIN_API_BASE_URL: "auth",
    VITE_USER_API_BASE_URL: "user",
    VITE_PARTNERSHIP_API_BASE_URL: "partnership",
    VITE_DOCUMENT_API_BASE_URL: "document",
    VITE_API_BASE_URL: "partnership",
  } as const;
  const proxy: Record<string, ProxyOptions> = {};

  // Prefix layanan menjaga rute yang sama tetap menuju backend yang tepat.
  for (const [service, target] of Object.entries(targets)) {
    const prefix = `/backend/${service}`;
    proxy[`${prefix}/`] = {
      target,
      changeOrigin: true,
      rewrite: (requestPath) => requestPath.slice(prefix.length),
    };
  }

  // Pertahankan rute relatif lama yang masih dipakai unggah/unduh dokumen.
  const legacyRoutes = {
    "/api/v1/auth": "auth",
    "/api/v1/admin": "auth",
    "/api/v1/profiles": "user",
    "/api/v1/dashboard": "user",
    "/api/v1/register": "user",
    "/api/v1/partnerships": "partnership",
    "/api/v1/mitra": "user",
    "/api/v1/umkm": "user",
    "/api/v1/documents": "document",
    "/api/v1/public/documents": "document",
  } as const;
  for (const [prefix, service] of Object.entries(legacyRoutes)) {
    proxy[prefix] = { target: targets[service], changeOrigin: true };
  }

  return {
    plugins: [tailwindcss(), react()],
    // Build produksi tetap memakai URL eksplisit dari lingkungan deployment.
    define: useDevProxy
      ? Object.fromEntries(Object.entries(apiServices).map(([variable, service]) => [
          `import.meta.env.${variable}`,
          JSON.stringify(`/backend/${service}/api/v1`),
        ]))
      : undefined,
    resolve: {
      alias: {
        react: path.resolve(__dirname, "node_modules/react"),
        "react-dom": path.resolve(__dirname, "node_modules/react-dom"),
      },
    },
    server: {
      port: 5173,
      strictPort: true,
      proxy,
    },
  };
});
