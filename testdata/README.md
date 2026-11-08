# testdata

| Path | Role |
|------|------|
| **`example/`** | Default dogfood: **depth over volume**. Multi-ledger, real-repo surface (includes, pad/balance/close, meta, docs). |
| **`kitchensink/`** | Scale corpus (~1M txns). Untouched by example depth work. |
| **`golden/`** | Reserved for expected snapshots. |

## `example/` (deep)

```bash
contapila -C testdata/example check   # expects OK + surface warnings where still unsupported
contapila -C testdata/example web
```

| Ledger | Exercises |
|--------|-----------|
| personal | Root commodities+prices includes; auto-loaded `indexes.beancount` (CDI); CDB/LCA `interest_rate`; cards; avg-cost equities; pad→balance; close temp; mirrored Acme distributions |
| acme | AR/AP; root includes; pad/balance; close clearing; invoice meta |
| ong | Grants deferred |
| smuggle | `CIGPK` inventory |

Also: `<ledger>/docs/by-account/…` (SPEC §4.4), CUE `links` (SPEC §4.5, not enforced), `query` (parsed; web_queries UI; no BQL yet). Prelude `project_journals` auto-loads root `prices.beancount` + `indexes.beancount` (regenerate indexes with `scripts/fetch-cdi` + `contapila ingest`).

## `kitchensink/`

High volume only — do not hand-edit for surface features.
