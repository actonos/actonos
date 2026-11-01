# web

React 19 + Vite + Tailwind v4 + i18next, hash routing (`#/route?query`). The daemon embeds the build from `internal/server/dist`.

## Rules that CI enforces

- eslint `--max-warnings=0`, no `any`, named exports only, props typed.
- Every visible string is a locale key in **both** `src/locales/en` and `vi` (`npm run check:i18n`, `npm run audit:i18n -- --fail`). New namespace: also `src/lib/i18n.ts`. Tests wait for `i18nReady` (already in `src/test/setup.ts`).
- No emoji in UI (`npm run check:emoji`); design tokens and geometry in `docs/DESIGN.md` (pill radius on controls, no drop shadows).
- Entry bundle budget: `web/scripts/check-bundle.mjs`; lazy-load pages.
- Types mirror Go JSON structs in `src/lib/types.ts`; API clients live in `src/lib/api/`.

## Working here

- Typecheck and test narrowly: `npx tsc --noEmit`, `npx vitest run src/pages/<Page>`; full gate `npm run quality` (run alone; Playwright times out under load).
- Playwright (`tests/*.spec.ts`) mocks the API with `page.route`; add a spec only for flows a unit test cannot cover. Visual baselines: update only for intentional UI changes.
- If `vite build` fails with a missing Tailwind/oxide binding, `node_modules` came from another OS: `npm ci`.
- New page checklist: page in `src/pages/<Name>/`, `App.tsx` route, `Sidebar.tsx`, `nav.json` (both locales), source-registry.
