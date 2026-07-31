package web

// AccountHref is the account page URL for templ outside package web
// (first-party plugins). Same as Session.AccountURL / accountURL helpers.
func AccountHref(s *Session, ledger, account, timeFilter string) string {
	return accountURL(s, ledger, account, timeFilter)
}
