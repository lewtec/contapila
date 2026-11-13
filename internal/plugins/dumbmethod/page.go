// Package dumbmethod is the dumb_method contribution planner: buy-only target
// allocation routing over user-defined asset classes and force scores.
package dumbmethod

import (
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/plugin"
	"github.com/lucasew/contapila-go/internal/web"
)

func init() {
	plugin.RegisterTyped(PluginKey, func(reg *plugin.Reg[Settings]) {
		reg.Page(web.Page{
			ID:      PageID,
			Label:   "Contributions",
			Order:   55,
			Sidebar: true,
			Build:   true,
			Fill:    fill,
			Body:    dumbMethodBody,
		})
	})
}

func fill(pc web.PageContext, data *web.PageData) {
	data.DumbMethod = buildReport(pc)
}

func buildReport(pc web.PageContext) *web.DumbMethodReport {
	rep := &web.DumbMethodReport{AsOf: asOfLabel(pc.AsOf)}
	if pc.Ledger == nil {
		rep.Warnings = append(rep.Warnings, "no ledger loaded")
		return rep
	}
	rep.OpCurrency = pc.Ledger.OpCurrency

	amount, err := parseAmount(amountQuery(pc.Request))
	if err != nil {
		rep.Warnings = append(rep.Warnings, err.Error())
		amount = new(big.Rat)
	}
	rep.Amount = money(amount)

	var cfg cue.Value
	if pc.Project != nil && pc.Project.Config != nil {
		cfg = pc.Project.Config.Value
	}

	classes, warns := classTargets(cfg)
	rep.Warnings = append(rep.Warnings, warns...)

	raw, hwarns, err := loadHoldings(pc.Ledger, pc.Prices, pc.AsOf, cfg)
	rep.Warnings = append(rep.Warnings, hwarns...)
	if err != nil {
		rep.Warnings = append(rep.Warnings, err.Error())
		return rep
	}

	classified, cwarns, err := classify(cfg, raw)
	rep.Warnings = append(rep.Warnings, cwarns...)
	switch {
	case errors.Is(err, ErrNoClassify):
		rep.Hint = "Add plugins.dumb_method.settings.#Classify to contapila.cue: declare holdings: [...#Holding] and classified: [for h in holdings {h & {...}}] with class, force, and id rules."
	case err != nil:
		rep.Warnings = append(rep.Warnings, err.Error())
	}

	inputs := make([]HoldingInput, 0, len(raw))
	for _, h := range raw {
		in := HoldingInput{
			Commodity: h.Commodity,
			Account:   h.Account,
			Units:     h.Units,
			Value:     h.Value,
			Price:     h.Price,
			Priced:    h.Price != nil,
			Precision: h.Precision,
		}
		if c, ok := classified[h.Index]; ok {
			in.Class = c.Class
			in.Force = c.Force
			in.HasForce = c.HasForce
			in.AssetID = c.ID
			if c.ID != "" {
				in.AssetLabel = c.ID
			}
		}
		if in.AssetID == "" {
			in.AssetID = h.Account + "|" + h.Commodity
		}
		if in.AssetLabel == "" {
			in.AssetLabel = h.Commodity
		}
		inputs = append(inputs, in)
	}

	plan := Compute(amount, classes, inputs)
	rep.Total = money(plan.Total)
	rep.NewTotal = money(plan.NewTotal)
	rep.Allocated = money(plan.Allocated)
	rep.Uninvested = money(plan.Uninvested)
	rep.Warnings = append(rep.Warnings, plan.Warnings...)

	for _, cp := range plan.Classes {
		rep.Classes = append(rep.Classes, web.DumbMethodClassRow{
			Class:      cp.ID,
			Target:     percentText(cp.Target),
			Current:    money(cp.Current),
			CurrentPct: percentText(percent(cp.Current, plan.Total)),
			Desired:    money(cp.Desired),
			Gap:        money(cp.Gap),
			Budget:     money(cp.Budget),
			Spent:      money(cp.Spent),
		})
	}
	for _, ap := range plan.Assets {
		rep.Assets = append(rep.Assets, web.DumbMethodAssetRow{
			Class:     ap.Class,
			Asset:     ap.Label,
			Commodity: ap.Commodity,
			Accounts:  strings.Join(ap.Accounts, ", "),
			Force:     forceText(ap.Force, ap.HasForce),
			Current:   money(ap.Current),
			Price:     money(ap.Price),
			Desired:   money(ap.Desired),
			Gap:       money(ap.Gap),
			Buy:       money(ap.Buy),
			Units:     unitsText(ap.Units, ap.Precision),
			Status:    ap.Status,
		})
	}
	for _, h := range plan.Unclassified {
		note := "no class"
		if h.Class != "" {
			note = fmt.Sprintf("unknown class %q", h.Class)
		}
		rep.Unclassified = append(rep.Unclassified, web.DumbMethodHoldingRow{
			Account:   h.Account,
			Commodity: h.Commodity,
			Units:     unitsText(h.Units, h.Precision),
			Value:     money(h.Value),
			Note:      note,
		})
	}
	return rep
}

func amountQuery(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.URL.Query().Get("amount")
}
