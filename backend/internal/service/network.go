package service

import (
	"context"
	"net"
	"net/url"
	"strings"
)

type requestBaseKey struct{}

// Accept only real DNS subdomains of the requested ngrok suffix. In
// particular, the apex, suffix lookalikes and arbitrary ports are not matches.
func ngrokBaseURL(host string) (string, bool) {
	host = strings.ToLower(host)
	if strings.Contains(host, ":") {
		name, port, err := net.SplitHostPort(host)
		if err != nil || port != "443" {
			return "", false
		}
		host = name
	}
	const suffix = ".ngrok-free.app"
	if !strings.HasSuffix(host, suffix) || len(host) <= len(suffix) || len(host) > 253 {
		return "", false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", false
			}
		}
	}
	return "https://" + host, true
}

func ngrokOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Opaque != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") || u.Path != "" && u.Path != "/" {
		return "", false
	}
	return ngrokBaseURL(u.Host)
}

func (s *Service) requestBase(host string) (string, bool) {
	localBase := strings.TrimRight(s.opt.LocalURL, "/")
	local, err := url.Parse(localBase)
	if err == nil && local.Host != "" && strings.EqualFold(host, local.Host) {
		return localBase, true
	}
	if base, ok := ngrokBaseURL(host); ok {
		return base, true
	}
	// Older isolated fixtures omit the listener URL; production never does.
	return "", s.opt.LocalURL == ""
}

func (s *Service) requestOriginAllowed(origin, base string) bool {
	if origin == "" {
		return true
	}
	if base == strings.TrimRight(s.opt.LocalURL, "/") {
		if origin == base {
			return true
		}
		_, ok := ngrokOrigin(origin)
		return ok
	}
	normalized, ok := ngrokOrigin(origin)
	return ok && normalized == base
}

func (s *Service) requestBaseURL(ctx context.Context) string {
	if base, ok := ctx.Value(requestBaseKey{}).(string); ok {
		return base
	}
	return strings.TrimRight(s.opt.LocalURL, "/")
}
