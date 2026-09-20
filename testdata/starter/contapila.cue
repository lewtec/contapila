commodities: {
	BRL: {precision: 2}
}

links: [
	{
		name: "company-aporte"
		from: {ledger: "company", account: "Equity:Aporte"}
		to:   {ledger: "personal", account: "Expenses:CustoFixo:PJ"}
		note: "Capital contribution: company equity matches personal PJ expense."
	},
	{
		name: "company-profit-distribution"
		from: {ledger: "company", account: "Equity:DistribuicaoLucros"}
		to:   {ledger: "personal", account: "Income:Ativo:BR:DistribuicaoLucros:Company"}
		note: "Profit distribution: company equity matches personal income."
	},
]
