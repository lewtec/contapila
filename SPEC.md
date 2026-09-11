# Contapila Specification

This document constrains contapila: a local one-binary Beancount-class bookkeeper with average-cost inventory, a multi-ledger project, and read-only HTML plus a lewkit `x/cmd` CLI.

Status: draft
Genre: app + cli

The key words MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY in this
document are to be interpreted as described in BCP 14 (RFC 2119,
RFC 8174) when, and only when, they appear in all capitals.

## Intention

Job: Load a conventional multi-ledger Project from plain-text journals. Run `check`. Answer balances, journal, P&L, and net worth from the CLI and from read-only local HTML (`web`, `desktop`, `build`). Expose the same Project truth through `lsp`. Book inventory at merged average-cost even when that disagrees with upstream Beancount.

Non-goals (this project):

1. Python Beancount at runtime.
2. User-loadable plugin code (shared objects, code the `plugin` directive fetches).
3. HTTP write-back of journals.
4. Multi-user accounts and remote multi-tenant hosting (a later platform project may own that).
5. `bean-*` flag compatibility.
6. A second binary.
7. FIFO, LIFO, STRICT multi-lot booking.
8. A CUE language server (cuepls stays separate).

Later work is the closed list at the end of this document. It is not in scope now.

Inherited C (cite the file):

| Binding | Cite |
|---------|------|
| Language Go 1.27 | `go.mod` |
| One lewkit `x/cmd` program | `cmd/contapila/main.go` |
| Project marker `contapila.cue`; ledgers `<root>/*/main.beancount` | `pkg/project/project.go` |
| Embedded CUE prelude | `internal/config/prelude.cue` |
| Average-cost inventory | `internal/booking/booking.go` |
| Desktop wrap eletrocromo, App.ID `br.tec.lew.contapila` | `cmd/contapila/desktop.go` |
| First-party modules | `internal/plugin/plugin.go` |
| Commands: `status`, `check`, `balances`, `journal`, `pnl`, `networth`, `account`, `parse`, `ingest`, `dump`, `web`, `build`, `desktop`, `lsp` | `cmd/contapila/main.go` |
| No database | this tree |
| Advertised hosts: linux, darwin | `README.md` |

## Technique

| ID | Input | Rule | Output |
|----|-------|------|--------|
| TEC-01 | A local process with no person accounts | Address the Project root and Ledger directory names. One process owns one Project. A Ledger is one economic subject (a person; a business). | One Project and its named Ledgers |
| TEC-02 | Journals, `contapila.cue`, and optional `<ledger>/docs/by-account` on disk | Walk up for the nearest marker. Discover one-level `*/main.beancount`. Resolve `include` against the including file. CLI and `web` reload from disk. `lsp` overlays open buffers. | Project, isolated Ledgers, shared PriceDB |
| TEC-03 | The same Ledger APIs the CLI uses | Render HTML on the server. Deliver those pages three ways: loopback HTTP, a dedicated app window, a static HTML tree. HTTP MUST NOT write journals. | HTML reports |
| TEC-04 | `web` against `desktop` | `web` uses no credentials. The app-window host issues a one-shot token and owns the loopback bind. A missing window host fails closed. | Local-only access |
| TEC-05 | argv | Run the frozen command set. When both stdin and stdout are not TTYs and argv matches the implicit-desktop table, rewrite to `desktop`. Discovery uses `-C` when set, else the process working directory. There is no `--config`. | One command runs |
| TEC-06 | A command result | Print a `Run` error on stderr and exit 1. `check` fails on errors. `check` succeeds when only warnings exist. Reports print human text on stdout. `lsp` uses stdout for the protocol only. | Unix exit status and one stdout shape |
| TEC-07 | An Operator write | Change journal bytes only through `ingest` (span surgery; upsert by `ingest_id`; append when that key is absent). `build` writes the `--out` directory. `dump` prints JSON on stdout. | Updated journal file, site tree, JSON |
| TEC-08 | Postings | Keep one merged average-cost Position per Account and commodity. An increase MUST carry a cost basis (braces win over `@` and `@@`). A reduction without braces books at the current average. A Transaction that does not balance MUST have exactly one empty residual posting. Oversell warns and MUST NOT invent units. Net worth uses PriceDB only. | Positions and diagnostics |
| TEC-09 | Embedded prelude, Operator `contapila.cue`, host-injected ledger and price-pair facts | Unify in CUE to one frozen RuntimeConfig. Transactions, pads, balances, and price time series stay out of that unify. First-party modules are gated by `plugins.<id>`. | RuntimeConfig |

## Tooling

| TEC | Tool | Relation | We do not | Cite |
|-----|------|----------|-----------|------|
| TEC-01 | — | none | A user table, OIDC, Backstage sessions | none |
| TEC-02 | Project discovery in this repo | implement | A database | `path:pkg/project` |
| TEC-02 | modernc Beancount grammar | wrap | A hand parser, Python Beancount, gobean | `path:internal/parser` |
| TEC-02 | `math/big.Rat` | adopt | `float64` for money | `stdlib:math/big` |
| TEC-03 | templ, daisyUI, tailgopher | adopt | An SPA as the page model | org:templ |
| TEC-03 | Pages in this repo | implement | A second page model beside templ | `path:internal/web` |
| TEC-03 | vendored uPlot | wrap | A second chart series API | `path:internal/web/static/vendor/uplot` |
| TEC-04 | eletrocromo | wrap | Embed Chromium. Fall back to the system browser | `lewtec/eletrocromo` |
| TEC-05 | lewkit `x/cmd` | adopt | `bean-*` flag clones | `path:cmd/contapila` |
| TEC-05 | `go.lsp.dev/protocol` + `jsonrpc2` | wrap | glsp, gopls `internal` | `path:internal/lsp` |
| TEC-05 | dslipak/pdf, excelize | wrap | A third PDF/XLSX stack | `path:internal/dump` |
| TEC-06 | lewkit `x/cmd`, `log/slog` | adopt | A second error facade | `path:cmd/contapila` |
| TEC-07 | Span surgery + temp/rename in this repo | implement | HTTP write-back | `path:internal/ingest` |
| TEC-08 | Booking in this repo | implement | Beancount lots, gobean ledger | `path:internal/booking` |
| TEC-09 | CUE | adopt | A Go “who wins” merge | `path:internal/config` |
| TEC-09 | First-party module registry | implement | User-loadable plugin code | `path:internal/plugin` |

| Cell | Pick | C or D | Implements | Cite if C |
|------|------|--------|------------|-----------|
| Language | Go 1.27 | C | TEC-02…TEC-09 | `go.mod` |
| Runtime | One OS process | C | TEC-01, TEC-05 | `cmd/contapila/main.go` |
| Persistence | Plain-text files | C | TEC-02 | `pkg/project/project.go` |
| UI | Server-rendered HTML | C | TEC-03 | `internal/web` |
| Packaging | One binary via goreleaser | C | TEC-05 | `mise.toml` |
| Identity | None | C | TEC-01 | none |
| Host OS | linux, darwin | C | TEC-04 | `README.md` |

## Terminology

| Concept | Approved | Banned |
|---------|----------|--------|
| The program | contapila | the binary, the tool, the engine (as a product name) |
| Person at the desk | Operator | user, customer, client |
| Marker directory | Project | workspace, repo, books (as the root) |
| One economic subject’s books | Ledger | book, company file, entrypoint |
| Named account | Account | bucket, category (as the type) |
| Currency / ticker | Commodity | currency (as the type name), asset class (as the type) |
| Booked inventory | Position | lot, lot list |
| Journal movement | Transaction | entry (as the type), txn in prose |
| One Transaction line | Posting | leg (except residual posting) |
| Frozen CUE snapshot | RuntimeConfig | config object, settings blob |
| First-party in-binary module | Module | user plugin, Python plugin |
| CUE map of Module flags | `plugins` | plugin system |
| Validation command | `check` | validate, verify, lint |
| Market conversion store | PriceDB | price cache, FX table |
| Read-only HTML on a port | `web` | Fava, server (as the command) |
| HTML in an app window | `desktop` | Electron, Wails |
| Static HTML export | `build` | generate, render site (as the command) |
| Language server command | `lsp` | contapila-lsp |
| Journal writer | `ingest` | import, merge (as the command) |
| Document tree dump | `dump` | extract (as the command) |

## Types

### Stored model (app)

| Entity | Kind | Identity authority | A/B rels `(min,max)` | Root | Invariant IDs |
|--------|------|--------------------|----------------------|------|---------------|
| Project | entity | Directory of the nearest `contapila.cue` | contains Ledger `(0,*)`; owns Commodity `(0,*)`; owns PricePoint `(0,*)` | yes | INV-01, INV-07, INV-08, INV-09 |
| Ledger | entity | Directory name under that Project. One economic subject (a person; a business). | owned by Project `(1,1)`; contains Account `(0,*)`; contains Transaction `(0,*)`; contains Document `(0,*)` | yes (inventory) | INV-02, INV-05, INV-06, INV-10 |
| Account | entity | `open` name inside that Ledger | owned by Ledger `(1,1)` | no | INV-03, INV-09 |
| Commodity | entity | Currency code, Project-shared (CUE ⊔ journal) | owned by Project `(1,1)` | no | INV-09 |
| Transaction | entity | `ingest_id` metadata when present; otherwise file path + source byte span | owned by Ledger `(1,1)`; contains Posting `(1,*)` | no | INV-04 |
| Posting | weak | (`Transaction`, line) | owned by Transaction `(1,1)` | no | INV-04 |
| Document | entity | Path under that Ledger (`docs/by-account`; explicit `document` directive) | owned by Ledger `(1,1)` | no | INV-07 |
| PricePoint | weak | (`base`, `quote`, date); last write wins | owned by Project `(1,1)` | no | INV-06 |

Values (no identity): Amount (`Rat` + Commodity), Position (`account`, commodity → units, total cost, cost commodity), RuntimeConfig, Diagnostic, LedgerLink (declared on Project; `check` does not reconcile), ModuleFlag (`plugins.<id>` on RuntimeConfig; IDs are compile-time).

Ban: a Person table. A second Commodity list in Go beside CUE. Invented `ledgers` keys in `contapila.cue`.

| Rel | A role | B role | A `(min,max)` | B `(min,max)` | Identifying? | Owner | Ban |
|-----|--------|--------|---------------|---------------|--------------|-------|-----|
| contains | Project | Ledger | `(0,*)` | `(1,1)` | no | host inject from `*/main.beancount` | user-authored `ledgers` keys |
| prices | Project | Commodity | `(0,*)` | `(1,1)` | no | RuntimeConfig ⊔ journal | per-Ledger precision |
| quotes | Project | PricePoint | `(0,*)` | `(1,1)` | yes | PriceDB | cost-basis rows in PriceDB |
| holds | Ledger | Account | `(0,*)` | `(1,1)` | no | that Ledger’s `open` | shared Account chart |
| books | Ledger | Transaction | `(0,*)` | `(1,1)` | no | that Ledger’s stream | merged multi-Ledger Transaction |
| files | Ledger | Document | `(0,*)` | `(1,1)` | no | that Ledger | Project-global docs |
| contains | Transaction | Posting | `(1,*)` | `(1,1)` | yes | Posting key includes Transaction | standalone Posting id |

### Commands (cli)

| Command | Type it mutates | Transition | Bad input |
|---------|-----------------|------------|-----------|
| `status` | none | Read Project | Not a Project → stderr, exit 1 |
| `check` | none | Read Ledger | Hard diagnostics → print, exit 1 |
| `balances` | none | Read Ledger | Unknown Ledger, bad `--as-of` → stderr, exit 1 |
| `journal` | none | Read Ledger | Unknown Ledger, bad time flags → stderr, exit 1 |
| `pnl` | none | Read Ledger | Unknown Ledger, bad time flags → stderr, exit 1 |
| `networth` | none | Read Ledger | Unknown Ledger, bad `--as-of` → stderr, exit 1 |
| `account` | none | Read Account | Unknown Ledger, unknown flags → stderr, exit 1 |
| `parse` | none | Read one file | Parse fail → stderr, exit 1 |
| `ingest` | journal file (Transactions) | Upsert by `ingest_id`; append when absent | Missing `--file`, parse fail → stderr, exit 1; file unchanged |
| `dump` | none | Read a source document → JSON stdout | Missing dialect/path, extract fail → stderr, exit 1 |
| `web` | none | Serve HTML | Bind fail → stderr, exit 1 |
| `build` | files under `--out` (not journals) | Write static HTML | Fail → stderr, exit 1 |
| `desktop` | none | Same handler in an app window | Helium / ensure / `Run` fail → stderr, exit 1 |
| `lsp` | none on disk | Overlay buffers in memory | Setup fail → stderr, exit 1 |

When a command takes `[ledger]` and the Operator names none, the command runs for every Ledger. Zero Ledgers on `check`, reports, `web`, `desktop`: error, exit 1.

## Invariants

| ID | Predicate | On | Forbidden bypass |
|----|-----------|----|------------------|
| INV-01 | The nearest `contapila.cue` walking up is the Project | Project | `--config`. A second root in one process |
| INV-02 | Inventories never merge across Ledgers | Ledger | A combined-books report that books together |
| INV-03 | At most one `open` per Account name in a Ledger | Account | A second `open` ignored |
| INV-04 | A Transaction that does not balance MUST have exactly one empty residual posting | Transaction | An implicit gains Account |
| INV-05 | One merged average-cost Position per (Account, commodity) | Ledger | Lot rows. FIFO. LIFO. STRICT |
| INV-06 | Net worth uses PriceDB only. A missing price is 0 plus a warning. Scope is Assets and Liabilities. Units keep their sign | Ledger | Cost-basis fallback. A second sign flip |
| INV-07 | HTTP and `lsp` do not write journal bytes | Project | In-browser save |
| INV-08 | Operator `contapila.cue` cannot invent `ledgers` keys | Project | Listing Ledgers by hand |
| INV-09 | Commodity policy is Project-shared. Account `open` / `close` are per Ledger | Project, Ledger | Per-Ledger Commodity precision |
| INV-10 | `check` fails only on errors. Warnings print. The command succeeds | Ledger | Warnings-as-errors as the default |

## Errors

| Public operation | Bad input | One reaction |
|------------------|-----------|--------------|
| Any CLI except `lsp` | Not a Project, unknown Ledger, bad flags/date, Helium/`Run` fail | stderr, exit 1 |
| `check` | Hard diagnostics | Print them, exit 1 |
| `check` | Warnings only | Print them, exit 0 |
| `ingest` | Absent `--file`. Unparseable input | stderr, exit 1; file unchanged |
| `dump` | Absent dialect. Absent path. Extract fail | stderr, exit 1 |
| `lsp` setup | stdio / server fail | stderr, exit 1; stdout unused |
| `textDocument/definition` | No `open` | Empty result |
| `textDocument/completion` | Position is not a completion slot. No snapshot | Empty list |
| `textDocument/hover` | Unknown non-Account token | Empty hover |
| `textDocument/hover` | Unknown Account | Thin “not opened” line |
| `textDocument/publishDiagnostics` | Parse fail | Publish parse diagnostics now. Keep last-good index and `check` diagnostics |
| GET `/l/{ledger}/…` | Unknown Ledger. Bad `?time=` | 400 |
| GET `/l/{ledger}/{page}` | Unknown page | 404 |
| GET `/l/{ledger}/account/…` | Bad path encoding | 400 |
| GET `/l/{ledger}/account/…` | Empty Account path | 404 |
| GET `/docfile/…` | Path outside that Ledger’s `docs/` | 404 |
| GET any HTML | Project load fail | 500 |
| `web` bind | Address in use | stderr, exit 1 |
| Ledger open / `check` | Unopened Account used | warn, allow |
| Ledger open / `check` | Posting after `close` | error |
| Ledger open / `check` | Duplicate `open` | error |
| Ledger open / `check` | Unbalanced Transaction, no residual posting | error |
| Ledger open / `check` | Failed `balance` assertion | error |
| Ledger open / `check` | Oversell | warn; skip inventing inventory |
| Ledger open / `check` | Explicit reduce cost ≠ current average beyond tolerance | error |
| Ledger open / `check` | Amount with number and no Commodity (not residual) | error |
| Ledger open / `check` | Invalid `interest_rate` on `open` | error |
| Ledger open / `check` | Unknown `option` | warn |
| Ledger open / `check` | `include` literal path missing | error |
| Ledger open / `check` | `include` glob, zero matches | warn |
| Ledger open / `check` | Include cycle | error |
| Ledger open / `check` | Double-include same realpath | skip (dedupe) |
| Ledger open / `check` | Missing `operating_currency` (inferred) | warn |
| Ledger open / `check` | Price missing for market conversion | warn; value 0 |
| Ledger open / `check` | `prices.beancount` empty/missing | warn |
| Ledger open / `check` | Unknown directive | warn and skip |
| Ledger open / `check` | Unknown `plugin "id"` | warn and skip |
| Ledger open / `check` | CUE unify failure | error |
| Ledger open / `check` | `closing: TRUE` with no inferable Commodity | error |
| Ledger open / `check` | `closing: TRUE` when `close` already exists | warn; skip synthetic `close`; still assert `balance 0` |

## Actors

| Actor | Obligations |
|-------|-------------|
| Operator | Keep journals as text. Run contapila on a local machine. Treat each Ledger as one economic subject |

## Capabilities

| ID | Actor | Sea-level goal |
|----|--------|----------------|
| CAP-01 | Operator | Open a Project |
| CAP-02 | Operator | Check the Project’s Ledgers |
| CAP-03 | Operator | Read balances, journal, P&L, net worth, and one Account |
| CAP-04 | Operator | Ingest directives into a journal file |
| CAP-05 | Operator | Dump a PDF/XLSX to a JSON tree |
| CAP-06 | Operator | Read the same reports as HTML (`web`, `desktop`, `build`) |
| CAP-07 | Operator | Edit journals in an editor via `lsp` |

## Public contract

### Project layout

Start at `-C` when set. Otherwise start at the process working directory. Walk up. The nearest `contapila.cue` is the Project root. An empty marker file is valid. Absence is not a Project.

Ledgers are exactly `<root>/*/main.beancount`. The Ledger name is the directory name. A directory without `main.beancount` is ignored. Recursive `**/main.beancount` is not a Ledger. A root-level `main.beancount` is not a Ledger.

`project_journals` in the prelude defaults to `prices.beancount` (`role: "prices"`, missing warn) and `indexes.beancount` (`role: "stream"`, missing ignore). Role `prices` fills PriceDB. Role `stream` is injected into every Ledger stream. The Operator MAY replace the whole list in `contapila.cue`.

Include paths are relative to the including file’s directory. Absolute paths are allowed. File identity for cycle and dedupe is the realpath.

On Project open the host injects a closed `ledgers` map. Operator `contapila.cue` MUST NOT add keys under `ledgers`.

### CLI flags

Global: `-C` / `--directory` (start directory for discovery). `-v` / `--verbose` (debug `slog` on stderr).

Reports: `--as-of YYYY-MM-DD` on `balances` and `networth` (empty means latest). `--time` (Fava-style period) on period reports. `--from` and `--to` (inclusive `YYYY-MM-DD`). The Operator MUST NOT pass `--time` together with `--from` / `--to`.

`ingest --file` is required. The file is created on success when missing.

`dump --password` unlocks an encrypted PDF/XLSX. The password MUST NOT appear in the JSON.

`web --addr` defaults to `127.0.0.1:8765`. `desktop` has no `--addr`.

`build -o` / `--out` defaults to `site`. `--jobs` `0` means `GOMAXPROCS`.

Implicit rewrite to `desktop` (TEC-05):

| argv when stdin and stdout are not TTYs | Becomes |
|------------------------------------------|---------|
| `contapila` | `desktop` at `-C` / cwd |
| `contapila /path/to/project` | work dir = that directory → `desktop` |
| `contapila /path/to/contapila.cue` | work dir = parent of the file → `desktop` |
| `contapila -C /path` | `desktop` |
| A real subcommand, two+ args, unknown junk | No rewrite |

A rewrite failure prints on stderr and exits 1. It MUST NOT fall through to help.

### HTTP

GET only for product pages. Routes:

- `/`
- `/l/{ledger}/` → check
- `/l/{ledger}/{page}` for registered pages (`check`, `balances`, `journal`, `pnl`, `networth`, `documents`, `prices`, debug `plugins` / `config`, plus enabled Module pages)
- `/l/{ledger}/account/{account}`
- `/l/{ledger}/commodity/{commodity}`
- `/l/{ledger}/query/{name}`
- `/docfile/{ledger}/docs/…` (that Ledger’s `docs/` only)
- `/static/…`

`web` reloads Project state from disk on every request. `build` writes extensioned `.html` files and MUST NOT write journals.

### LSP

Same binary. stdio. One Project per process (first resolved `contapila.cue` wins). Open buffers overlay disk. Closed files are read from disk on the next recompute.

Dogfood cut: `publishDiagnostics` (parse immediately; `check` after a successful parse, atomic snapshot swap), `definition` (Account → that Ledger’s `open`), `completion` (Account, Commodity, date slots), `hover` (Account `open` facts; Commodity policy). No live balances on hover.

Cancel in-flight slow work when a newer edit arrives. A broken parse MUST NOT extract symbols from the partial tree.

### Directives

| Directive | This version | Plane |
|-----------|--------------|-------|
| `option` | yes | → CUE |
| `include` (globs) | yes | Go load |
| `commodity` | yes | → CUE |
| `open` / `close` | yes | → CUE (per Ledger) |
| `*` / `!` Transaction, Postings | yes | Go |
| metadata on `open` / `commodity` | yes | Go + CUE |
| metadata on `price`, `balance`, `event`, Transaction, Posting | yes | Go (not CUE, except `open` / `commodity`) |
| org-mode `section` / headlines | structure only; silent | Go |
| posting `closing: TRUE` | yes — after residual fill, `balance 0` + `close` next day | Go |
| cost `{}`, price `@` / `@@` | yes | Go |
| cost `{amount, date}` | yes — books cost; injects `price` on that date | Go |
| amount expressions | yes | Go |
| empty residual posting | yes | Go |
| `price` | yes | Go PriceDB; CUE `price_pairs` inventory only |
| `balance`, `pad`, `note`, `event`, `document`, `custom`, `query` | yes | Go (`query` stored, not executed) |
| `custom "index"` | yes — autointerest projection | Go |
| `plugin "id"` | names a first-party Module for that Ledger open | Go; MUST NOT mutate `contapila.cue`; MUST NOT load outside code |
| `pushtag` / `poptag` / `pushmeta` / `popmeta` | no | — |

### Booking (TEC-08)

Apply directives by date ascending, then type rank (`open` → `pad` → `balance` → Transaction / note / event → `document` → `close`), then source line.

Increases: `{...}` wins over `@` / `@@`. `@` is unit cost. `@@` is total / units. When the Account already holds that commodity with a cost basis and braces are omitted, new units book at the current average. New units merge into that average.

Reductions: omitted braces book at the current average. Prefer `@@` total proceeds. One Posting per commodity.

FX cash spend: reducing a currency held at a foreign cost, to pay legs that need that currency in face terms, weights the cash Posting in the face commodity for balancing, and still reduces the foreign cost basis. Residual cash on a costed FX Account reduces inventory. Pure conversion to the operating currency plus gains weights by cost basis.

Autointerest: on `open` with `interest_rate` (alias `interest-rate`), parse the expression, counterpart `Assets:…` → `Income:Passivo:…`, materialize a `pad` the day before `balance` (skip when a pad exists), and on `close` inject `pad` + `balance 0` before close. Projection uses `custom "index"` through `time.Now()` and stops on `close`.

### Operating currency and prices

Prefer explicit `operating_currency` in CUE / `option`. When missing: warn, then take the commodity from the first Transaction that carries a posting amount commodity. When still none: currency-denominated reports error at report time.

Price lookup for as-of date D (PriceDB only):

1. Direct pair `base→quote` with date ≤ D.
2. Inverse of `quote→base`.
3. One intermediate hop. Resolve each leg with the stored pair. When that pair is missing, use the inverse. Both legs ≤ D.
4. Else warn; market value is 0.

Do not use a future price for a past as-of. Do not use Position cost as market value.

### Documents

Walk `<ledger>/docs/by-account/**`. Account components are directories (`:` → `/`). Filename prefix is one of `yyyy`, `yyyymm`, `yyyymmdd` (contiguous digits, then a non-digit or end of name). Omitted month or day defaults to `01`. Invalid calendars and other digit lengths are an error diagnostic; those files are skipped. Explicit `document` directives merge in; the same path prefers the explicit directive. Transaction / posting metadata `document:` expands into the Ledger document list at open.

### Config plane (TEC-09)

Embed CUE. Unify prelude, Operator `contapila.cue`, and host-injected facts. Unify failure is a load error.

In CUE: options, Commodity policy, `price_pairs` inventory, per-Ledger `open` / `close` facts, `plugins`, `links`, `project_journals`.

Not in CUE: Transactions, Postings, `balance`, `pad`, `note`, `event`, price time series, include resolution, Transaction metadata.

Default Commodity precision is 5. Default tolerance is half ULP of precision. An optional `tolerance` field overrides. Beancount `inferred_tolerance_*` options are not read. Undeclared commodities use prelude defaults.

A journal `plugin "id"` that names a known Module enables that Module for that Ledger open. It MUST NOT write `contapila.cue`. An unknown id warns and skips.

## Quality

| Concern | Measure, or why it cannot happen |
|---------|----------------------------------|
| Security | `web` binds `127.0.0.1:8765` by default. `/docfile` serves only `<ledger>/docs/**`. Desktop: eletrocromo token and library-owned bind. Missing Helium fails closed. HTTP does not write journals. |
| Identity / auth | This project has no person accounts. A Ledger directory is the handle for one economic subject. `web` has no credentials. Multi-user belongs to a later platform project. |
| Persistence | Journals and `contapila.cue` on disk are the books. CLI and `web` reload from disk. `lsp` overlays open buffers. Disk wins for closed files. No database. |
| Exit contract | `Run` error → exit 1. `check` exits 1 only on errors. `lsp` stdout is protocol only. |
| Untrusted input | Journals, ingest JSONL, dump files, and URL paths are untrusted. Fail closed on path escape, CUE unify failure, and booking constructs that would lie about balances. Unknown directives warn and skip when safe. |

## Security

In scope: loopback bind, desktop one-shot token, `/docfile` confinement, no journal writes from HTTP, fail-closed desktop host.

Why person-auth cannot happen: this project is a local single-Operator program. A platform project may add it later.

Residual risk: any local process can call `web` on the loopback port. Desktop token auth does not apply to `web`.

## Success

- [ ] Two Ledgers in one Project keep separate inventories for the same Account name.
- [ ] A file that never sets a booking method books average-cost, even when Beancount would use lots.
- [ ] HTTP GET cannot change journal bytes.
- [ ] `contapila web` serves on loopback without credentials.
- [ ] `desktop` without Helium exits 1 and does not open a system browser.
- [ ] `ingest` with the same `ingest_id` replaces the prior directive and leaves the rest of the file intact.
- [ ] `check` with only warnings exits 0.
- [ ] An unbalanced Transaction without a residual empty posting fails `check`.
- [ ] Oversell warns and does not create units.
- [ ] Operator `contapila.cue` that adds a `ledgers` key fails unify.
- [ ] `lsp` definition on an Account with no `open` returns empty.
- [ ] Net worth of an unpriced Position is 0 with a warning.

## Later work

1. BQL / `bean-query` execution (`query` directive is stored and shown only).
2. `contapila init` (copy an embedded fixture directory).
3. `option "booking_method"` / FIFO / LIFO / STRICT lots.
4. `check` reconciling LedgerLink balances.
5. In-browser journal edit / write-back.
6. Full CUE language server.
7. LSP beyond the dogfood cut: Commodity goto, references, rename, format, code actions, semantic tokens, workspace symbols, rich hover, required `fsnotify`.
8. Advertising Windows and other unadvertised hosts as supported.
9. A platform / multi-user product (different repo).

## Assumptions

| ID | Fact | If false |
|----|------|----------|
| AS-01 | The modernc Beancount grammar accepts the directive set in this document | Parser wrap fails; stop and change the grammar, not a hand parser |
| AS-02 | eletrocromo ensure can install Helium on advertised hosts | `desktop` fails closed; do not open the system browser |

## Decision history

- Average-cost is the inventory law. Rejected: Beancount lot default as this product’s default.
- First-party Modules stay in this binary. Rejected: user-loadable plugin code. Retracted: “plugins never”.
- Config unify is CUE. Rejected: YAML plus a Go merge table.
- Desktop window is eletrocromo. Rejected: system-browser fallback.
- Booking is this repo. Rejected: gobean ledger, Python Beancount at runtime.
- Genre is app + cli. Rejected: `pkg/*` as a supported import API.
