// Deep example — anonymized BR topology, multi-ledger, compact journals.
//
// Ledger set is injected from filesystem lookup of */main.beancount
// (see prelude #Ledger / #LedgerName). Do not list ledgers here.

commodities: {
	// B3 equities / FIIs: whole shares
	[=~"^B3_"]: {precision: 0}

	BRL: {precision: 2}
	USD: {precision: 2}
	TDBR_SELIC_2029: {precision: 2}
	BTC: {precision: 8}
	SPDW: {precision: 3}
	CIGPK: {precision: 0}
}

// Cross-ledger hints; ledger fields must be #LedgerName (discovered keys).
// Not enforced by check yet — CUE validates names only.
links: [
	{
		name: "acme-profit-distribution"
		from: {ledger: "acme", account: "Equity:DistribuicaoLucros"}
		to:   {ledger: "personal", account: "Income:Ativo:BR:DistribuicaoLucros:Acme"}
		note: "Company distributions should match personal income credits (same dates/amounts by convention)."
	},
]
