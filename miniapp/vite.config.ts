import { defineConfig } from "vite";
import uni from "@dcloudio/vite-plugin-uni";
import { fileURLToPath } from "node:url";
export default defineConfig({
  publicDir: process.env.UNI_PLATFORM === "h5" ? fileURLToPath(new URL("./public", import.meta.url)) : undefined,
  plugins: [uni()],
  server: { host: "127.0.0.1", proxy: { "/api": "http://127.0.0.1:18082" } },
});
