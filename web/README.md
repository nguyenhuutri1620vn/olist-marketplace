# web

The Olist Marketplace SPA: React 19, Vite 8, Tailwind 4 and shadcn/ui. It talks only to api-gateway through `/api/` on its own origin.

```bash
npm ci          # Node 24 / npm 11 (adding packages with npm 10 hits an npm bug: use `npx npm@11 install`)
npm run dev     # http://localhost:5173, proxies /api to the gateway from `make up`
npm test
npm run build
```

In `make up` the `web` container serves the built app with nginx on `127.0.0.1:5173` and proxies `/api/` to `api-gateway:8080`.
