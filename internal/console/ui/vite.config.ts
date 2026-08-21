import path from "node:path"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig } from "vite"

// The Go binary embeds dist/ and serves it from /, so assets must be
// root-relative and the output must stay inside this directory — nothing here
// is uploaded anywhere, it is compiled into the executable.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/",
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    // `npm run dev` talks to a console started separately, so the API is not
    // on Vite's port. Same-origin in production, proxied here.
    proxy: { "/api": "http://127.0.0.1:7673" },
  },
})
