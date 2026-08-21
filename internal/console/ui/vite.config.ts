import { writeFileSync } from "node:fs"
import path from "node:path"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig, type Plugin } from "vite"

// dist/.gitkeep is committed so //go:embed always has something to match and
// `go build` works in a fresh clone with no node toolchain. But emptyOutDir
// deletes it on every build, and the damage is invisible until someone clones
// the repo and the Go build fails on a pattern matching no files. Put it back
// where it was removed rather than asking anyone to remember.
function keepEmbedSentinel(): Plugin {
  return {
    name: "keep-embed-sentinel",
    closeBundle() {
      writeFileSync(path.resolve(import.meta.dirname, "dist/.gitkeep"), "")
    },
  }
}

// The Go binary embeds dist/ and serves it from /, so assets must be
// root-relative and the output must stay inside this directory — nothing here
// is uploaded anywhere, it is compiled into the executable.
export default defineConfig({
  plugins: [react(), tailwindcss(), keepEmbedSentinel()],
  base: "/",
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    // `npm run dev` talks to a console started separately, so the API is not
    // on Vite's port. Same-origin in production, proxied here.
    proxy: { "/api": "http://127.0.0.1:7673" },
  },
})
