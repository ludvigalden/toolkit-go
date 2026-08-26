// Package ssrf validates URLs before a server-side fetch: only http
// and https, every resolved address public. A later dial re-resolves
// DNS and can rebind; fetchers should dial through Validator.DialContext.
package ssrf
