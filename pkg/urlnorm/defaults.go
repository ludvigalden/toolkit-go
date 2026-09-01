package urlnorm

// DefaultTrackingParams returns the built-in marketing and analytics
// parameters (UTM, ad-click identifiers, email and social referrers).
// The slice is fresh on every call.
func DefaultTrackingParams() []string {
	return []string{
		"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id", "utm_referrer",
		"fbclid", "fb_action_ids", "fb_action_types", "fb_ref", "fb_source",
		"gclid", "gclsrc", "msclkid", "ttclid", "mc_cid", "mc_eid", "yclid",
		"li_fat_id", "twclid", "epik", "pp",
		"ref", "ref_src", "ref_url", "refref",
		"_hsenc", "_hsmi", "hsCtaTracking", "__hssc", "__hstc", "__hsfp",
		"vero_id", "igshid", "wickedid", "wickedsource",
		"oly_enc_id", "oly_anon_id", "marketing_id", "sr_share",
	}
}

// DefaultSessionParams returns the built-in session-identifier and
// anti-CSRF token parameters. The slice is fresh on every call.
func DefaultSessionParams() []string {
	return []string{
		"PHPSESSID", "osCsid", "SID",
		"csrf_token", "csrfmiddlewaretoken", "authenticity_token",
		"_wpnonce", "_wp_http_referer",
	}
}

// DefaultHostRules returns the built-in host rules for video, forum,
// wiki, and storefront URLs. The slice is fresh on every call.
func DefaultHostRules() []HostRule {
	return []HostRule{
		{Domains: []string{"youtube.com", "youtu.be"}, Drop: []string{"t", "list", "index"}},
		{Domains: []string{"reddit.com"}, Drop: []string{"context", "limit", "sort"}},
		{Domains: []string{"wikipedia.org", "wikimedia.org"}, Drop: []string{"oldid", "diff", "direction", "preload", "printable"}},
		{Domains: []string{"myshopify.com", "shopify.com"}, Drop: []string{"variant", "currency"}},
		{PathHints: []string{"viewtopic.php", "viewforum.php", "memberlist.php"}, Drop: []string{"sid"}},
	}
}
