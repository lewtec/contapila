# Contapila web UI

Status: density shell + uPlot charts + Cmd+K jump palette (reports, ledgers, accounts, commodities, queries).

## Intent

Fix **generic daisyUI** look and **too much air**. Keep working light/dark toggle.  
Composition: **Fava-like data/reports/time** + **Linear/Raycast-class density** for chrome and keyboard (Cmd+K).

## Personality (visual)

Quiet · Precise · Helix-ready · money-tool, not marketing.

**Mood:** AUVP-like **dark green + gold**, subtle Hermes-dashboard craft later — **token sprinkles only** in this pass.

## Color

| Role | Use |
|------|-----|
| **Dark green** | Primary: active nav, links, focus, chart series base |
| **Gold** | Rare accent (≤10%): key totals, Cmd+K selection highlight |
| **Base** | Near-pure neutral surfaces (not cream SaaS); dark = deep green-black tint |
| **Semantic** | error / warning / success / info for check only |

Light: green as primary on white/near-white bases — **not** a green page wash.  
Dark: deep base, muted gold for contrast.

Implementation (daisyUI way):

- Source: `styles/input.css` with `@plugin "daisyui"` and `@plugin "daisyui/theme"` for **`contapila-light`** / **`contapila-dark`**
- Build: `bun install && bun run build:css` (mise provides `bun`) → `internal/web/static/app.css`
- Serve: embedded `/static/app.css` (not CDN `themes.css` / browser Tailwind)
- Toggle: `data-theme="contapila-light"` | `contapila-dark` + `theme-controller`

## Typography

- System UI stack
- **Dense** scale: page titles ~1.125rem–1.25rem, not display sizes
- **Tabular nums** for all money (`tabular-nums` / `.tabular`)
- Account names: `font-mono` at compact size
- Breadcrumb labels use report names (Income statement, not “pnl”)

## Layout

```
┌─ sticky filter bar ──────────────────────────────┐
│ ☰  contapila › [ledger ▾] › page    [time]  ☀   │
├────────┬─────────────────────────────────────────┤
│ Reports│  full-bleed main (no floating max-width)│
│ …      │  tight page header + dense tables       │
└────────┴─────────────────────────────────────────┘
```

- Left **reports rail** (~13rem), menu-sm, minimal brand block
- **Ledger selector in breadcrumbs** (not a separate end control)
- **Time** = single Fava expression field
- Main content **full width** of drawer content (drop decorative max-width padding sea)
- Mobile: drawer + hamburger

## Density rules

- Prefer `table-xs` / `table-sm`, less `rounded-box` theater
- Page header margin small; section titles uppercase compact
- Journal: compact rows, not large cards
- Soft badges sparingly; prefer text + color for severity

## Components

navbar/topbar, drawer, menu, table, alert, badge, breadcrumbs, input, theme-controller,  
Cmd+K via reusable `pkg/cmdpalette`

## Charts (uPlot, vendored)

- Assets: `internal/web/static/vendor/uplot/` — update with `./scripts/vendor-uplot.sh [ver]`
- Glue: `static/charts.js` + templ components in `charts.templ` (`chartAssets`, `chartPanel`)
- **Net worth** / **account** / prices: spline-smoothed line through event samples (uPlot `paths.spline`), **op currency**, price ≤ event date
- **Income statement**: diverging bars (income up, expenses down); bin from time filter (year→month, month→day, multi-year→year)
- Hierarchy/treemap: not yet (can add another lib later without rewriting series APIs)

## Cmd+K

- **Reusable UI:** `pkg/cmdpalette` — `Palette` + `Button()` / `Modal()` / `Assets()` (templ + embedded CSS/JS)
- **Contapila catalog:** `commandItems` / `jumpPalette` in `internal/web` — ledgers, reports, queries, accounts, commodities, `PluginCommands`
- **Plugins:** `reg.Palette(web.Palette{DefaultEnabled, Fill})` (plugin hook, not the UI type). Example: `web_queries` “All queries”
- **Theme bridge:** CSS vars `--cmdpalette-accent` etc. set from contapila tokens in `styles/input.css`
- **Not yet:** slash actions (`time …`), recents, fuzzy rank beyond substring tokens

## Motion

Minimal 150–200ms state only; no page-load choreography.

## Anti-patterns (do not ship)

- Glassmorphism, gradient text, hero KPI theater
- Gold headings / gold everywhere
- Cream/sand body backgrounds
- Fat left accent borders on every card
- Airy marketing card grids
