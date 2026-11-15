---
name: web-ui-engineering
description: Use when implementing or changing GoGoMio browser behavior, API integration, UI modules and tests, or embedded web assets.
---

# Web UI Engineering

Use the TypeScript sources and focused browser tests as the source of truth. The Go server embeds the UI, so source changes, generated assets, and content hashes must stay aligned.

## Source and build workflow

- Browser behavior lives in [`internal/web/src/`](../../../internal/web/src/); page markup, styles, and design tokens live in [`internal/web/index.html`](../../../internal/web/index.html).
- Keep logic in focused source modules when a behavior already has a natural module or test boundary. Keep API requests in `api.ts` and follow existing response/error handling patterns.
- Do not edit [`internal/web/generated/app.js`](../../../internal/web/generated/app.js) directly. After changing source or assets, run `npm run build:web`; it regenerates sprite metadata, bundles the app, and updates content-versioned URLs.
- The repository pins Node through [`.nvmrc`](../../../.nvmrc). Use `npm ci` when setting up dependencies and respect the package's Node engine requirement.
- For visual redesigns, also follow [Front-end Design](../front-end-design/SKILL.md). For a small functional change, preserve the current layout and design tokens unless the brief asks for a redesign.

## Behavior and accessibility

- Preserve keyboard operation, semantic controls, visible focus, contrast, reduced-motion behavior, and narrow viewport usability when changing affected interactions.
- Reuse existing loading, error, status, notification, and polling patterns where they fit. Do not add a dependency or a second state mechanism without a concrete need.
- Keep API expectations aligned with the server contract. If routes or response shapes change, update the API/settings skill's OpenAPI and compatibility guidance as well.
- Add or update focused tests beside the source module for parsing, presentation, polling, retries, or interaction logic. Prefer deterministic tests over timing-dependent browser behavior.

## Verification

Run the web type checks, unit tests, and generated asset consistency check, then verify the Go embedding layer:

```bash
npm run check:web
go test ./internal/web
```

If the generated bundle or content hashes are stale, run `npm run build:web` and review all generated changes with the source diff.

## Related files

- [`internal/web/src/app.ts`](../../../internal/web/src/app.ts) — browser application entry point
- [`internal/web/src/api.ts`](../../../internal/web/src/api.ts) — API client
- [`internal/web/src/`](../../../internal/web/src/) — focused UI modules and tests
- [`internal/web/web.go`](../../../internal/web/web.go) — embedded static files and caching behavior
- [`scripts/check_web_assets.ts`](../../../scripts/check_web_assets.ts) — generated bundle and content-hash verification
