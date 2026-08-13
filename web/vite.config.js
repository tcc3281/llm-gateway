import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
// The SPA is embedded into the Go admin server via internal/web (go:embed),
// so the production build is written one level up into ../internal/web/dist.
// In dev, Vite proxies /admin API calls to the running Go server on :8081.
export default defineConfig({
    plugins: [react()],
    build: {
        outDir: "../internal/web/dist",
        emptyOutDir: true,
    },
    server: {
        port: 5173,
        proxy: {
            "/admin": "http://localhost:8081",
        },
    },
});
