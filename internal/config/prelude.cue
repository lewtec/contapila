// Commodity policy (CUE) — journal `commodity` directives may carry matching
// string attributes (name, asset-class, …). Host may inject discovered commodities later.
#Commodity: {
	// Decimal places for display / default tolerance (half ULP = 0.5 * 10^-precision).
	precision: int | *5
	// Optional absolute tolerance override (string or number, e.g. 0.001 or "0.001").
	// When unset, booking uses half ULP of precision.
	tolerance?: number | string
	// Common Beancount-style attributes (optional; open for more via ...).
	name?:         string
	"asset-class"?: string
	"further-info"?: string
	...
}

// Account policy / open facts (CUE). Journal `open` metadata unifies with this shape
// when the host injects ledgers.<name>.accounts (planned); authoring in contapila.cue is allowed.
#Account: {
	name: string
	// Declared currencies on `open Account CUR …`
	currencies?: [...string]
	// Common open metadata keys
	institution?: string
	role?:        string
	"asset-class"?: string
	"asset_class"?: string
	// Autointerest: canonical snake_case. Expression e.g. "115% CDI", "IPCA + 10% aa".
	interest_rate?: string
	// Journal / Beancount hyphen alias — unified onto interest_rate when set.
	"interest-rate"?: string
	if "interest-rate" != _|_ {
		interest_rate: "interest-rate"
	}
	...
}

// Filesystem-ish ledger directory name (parent of main.beancount).
#LedgerID: string & =~"^[A-Za-z][A-Za-z0-9_-]*$"

// One discovered ledger (injected by the host after scanning the project root).
#Ledger: {
	name: #LedgerID
	main: string // absolute path to main.beancount
	// Optional: account opens for this ledger (host inject and/or user overlays).
	accounts?: [Name=string]: #Account & {name: Name}
}

// Map of discovered ledgers. The host injects a *closed* concrete struct:
//
//	ledgers: close({
//	  personal: {name: "personal", main: ".../personal/main.beancount"}
//	})
//
// Open typing here; closedness comes from the generated fragment.
ledgers: [Name=string]: #Ledger & {
	name: Name
}

// Valid ledger name = a key of the injected ledgers map.
#LedgerName: or([for n, _ in ledgers {n}])

#LedgerRef: {
	ledger:  #LedgerName
	account: string
}

// Declarative cross-ledger reconciliation hint (not enforced by check yet).
#LedgerLink: {
	name:  string
	from:  #LedgerRef
	to:    #LedgerRef
	note?: string
}

// One base/quote pair observed in prices.beancount (pair inventory only).
// Full time series stay in Go PriceDB — not injected into CUE (volume).
// Host injects closed map keyed by "base|quote"; user may overlay policy fields.
#PricePair: {
	base:  string
	quote: string
	// Optional pair-level notes / source labels (user or later inject).
	source?: string
	note?:   string
	...
}

// One project-root beancount file the host auto-loads (paths relative to contapila.cue).
#ProjectJournal: {
	path: string
	// prices → shared PriceDB (not injected into ledger streams)
	// stream → prepended into every ledger directive stream
	role: "prices" | "stream"
	// when the file is absent: warn (slog) or ignore
	missing: "warn" | "ignore" | *"ignore"
}

#Config: {
	commodities: [string]: #Commodity
	operating_currency?: [...string]
	ledgers: [string]: #Ledger
	links?:  [...#LedgerLink]
	price_pairs?: [string]: #PricePair
	// Auto-imported root journals (override wholesale in contapila.cue if needed).
	project_journals: [...#ProjectJournal]
}

// Default instance fields (unified with host inject + user contapila.cue).
commodities: [string]: #Commodity
links?:      [...#LedgerLink]
price_pairs: [string]: #PricePair

// Default auto-imports — replace the whole list in contapila.cue to customize.
project_journals: [...#ProjectJournal] | *[
	{path: "prices.beancount", role: "prices", missing: "warn"},
	{path: "indexes.beancount", role: "stream", missing: "ignore"},
]
