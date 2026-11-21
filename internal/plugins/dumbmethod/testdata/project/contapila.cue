commodities: {
	BRL: {precision: 2}
	[=~"^B3_"]: {precision: 0}
}

project_journals: [
	{path: "prices.beancount", role: "prices"},
]

plugins: dumb_method: {
	enabled: true
	settings: {
		classes: {
			acoes: {target: 50}
			fiis:  {target: 50}
		}
		#Classify: {
			holdings: [...#Holding]
			classified: [for h in holdings {
				h & {
					if h.account == "Assets:BR:B3:Acao" {
						class: "acoes"
						force: 8
						id:    "PETR4"
					}
					if h.account == "Assets:BR:B3:FII" {
						class: "fiis"
						force: 5
						id:    "MXRF11"
					}
				}
			}]
		}
	}
}
