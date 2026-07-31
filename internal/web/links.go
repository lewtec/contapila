package web

// Public URL helpers for first-party plugins (and any templ outside package web).
// All honor Session.Static so contapila build gets .html / index.html hrefs.

// HomeHref is the site root.
func HomeHref(s *Session) string {
	return homeURL(s)
}

// LedgerHref is a ledger report page (/l/{ledger}/{page}).
func LedgerHref(s *Session, ledger, page, timeFilter string) string {
	return ledgerURL(s, ledger, page, timeFilter)
}

// LedgerRootHref is the ledger landing path (/l/{ledger}/).
func LedgerRootHref(s *Session, ledger string) string {
	if s == nil {
		return (&Session{}).LedgerRootURL(ledger)
	}
	return s.LedgerRootURL(ledger)
}

// AccountHref is an account page link.
func AccountHref(s *Session, ledger, account, timeFilter string) string {
	return accountURL(s, ledger, account, timeFilter)
}

// CommodityHref is a commodity page link.
func CommodityHref(s *Session, ledger, commodity, timeFilter string) string {
	return commodityURL(s, ledger, commodity, timeFilter)
}
