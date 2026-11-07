package qqcommunity

import (
	"context"
	"net"
	"net/url"
	"strings"
)

// User-facing links use the saved frontend URL, never the loopback renderer.
func (s *Service) publicSiteURL(ctx context.Context, path string) string {
	if s.bindingEmail == nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(s.bindingEmail.FrontendURL(ctx)), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return ""
	}
	return base + path
}
