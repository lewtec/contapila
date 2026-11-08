package web

import "github.com/lucasew/contapila-go/internal/booking"

func journalPostingHighlight(p booking.FilledPosting, d PageData) bool {
	if d.AccountName != "" && p.Account == d.AccountName {
		return true
	}
	if d.CommodityName != "" && p.Units != nil && p.Units.Commodity == d.CommodityName {
		return true
	}
	return false
}
