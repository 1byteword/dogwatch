import { defineConfig } from "vite";
import solidPlugin from "vite-plugin-solid";

export default defineConfig({
  plugins: [solidPlugin()],
  server: {
    port: 5174,
    proxy: {
      "/api": {
        target: "http://localhost:9999",
        changeOrigin: true
      },
      "/health": {
        target: "http://localhost:9999",
        changeOrigin: true
      },
      "/readyz": {
        target: "http://localhost:9999",
        changeOrigin: true
      }
    }
  },
  build: {
    target: "esnext"
  }
});
