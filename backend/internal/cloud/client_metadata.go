package cloud

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func base64Challenge(s string) ([]byte, error) {
	b, e := base64.RawURLEncoding.DecodeString(s)
	if e != nil || len(b) != 32 {
		return nil, errors.New("invalid S256 challenge")
	}
	return b, nil
}
func (o *OAuth) client(ctx context.Context, id string) (clientRecord, error) {
	if c, ok := o.store.data.Clients[id]; ok {
		return c, nil
	}
	u, e := url.Parse(id)
	if e != nil || !validHTTPS(id) || u.Path == "" || u.Path == "/" || u.RawQuery != "" || u.Port() != "" && u.Port() != "443" {
		return clientRecord{}, errors.New("unregistered client")
	}
	if len(o.store.data.Clients) >= 256 {
		return clientRecord{}, errors.New("client limit reached")
	}
	// Resolve and validate at dial time so metadata cannot reach localhost, LAN,
	// link-local metadata services, or rebound DNS addresses. Never follow redirects.
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		addresses, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		for _, a := range addresses {
			if !publicIP(a.IP) {
				return nil, errors.New("client metadata must be publicly hosted")
			}
		}
		if len(addresses) == 0 {
			return nil, errors.New("client metadata host is unavailable")
		}
		d := net.Dialer{Timeout: 5 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("metadata redirects are not allowed") }}
	req, e := http.NewRequestWithContext(ctx, "GET", id, nil)
	if e != nil {
		return clientRecord{}, e
	}
	req.Header.Set("Accept", "application/json")
	res, e := client.Do(req)
	if e != nil {
		return clientRecord{}, errors.New("client metadata could not be verified")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return clientRecord{}, errors.New("client metadata unavailable")
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 65537))
	if e != nil || len(b) > 65536 {
		return clientRecord{}, errors.New("client metadata too large")
	}
	var c clientRecord
	if json.Unmarshal(b, &c) != nil || c.ID != id || len(c.Redirects) == 0 || len(c.Redirects) > 8 || len(c.Name) > 120 || c.AuthMethod != "" && c.AuthMethod != "none" && !contains(c.AuthMethods, "none") {
		return clientRecord{}, errors.New("unsupported client metadata")
	}
	for _, r := range c.Redirects {
		if !validHTTPS(r) {
			return clientRecord{}, errors.New("invalid client redirect")
		}
	}
	c.AuthMethod = "none"
	o.store.data.Clients[id] = c
	if e = o.store.save(); e != nil {
		delete(o.store.data.Clients, id)
		return clientRecord{}, e
	}
	return c, nil
}
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	// Also exclude shared address space, benchmark/documentation/reserved networks.
	for _, block := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		_, n, _ := net.ParseCIDR(block)
		if n.Contains(ip) {
			return false
		}
	}
	return !strings.Contains(ip.String(), "%")
}
