package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// applicationURL requires the origin spelling browsers use. In particular,
// :443 and an omitted HTTPS port cannot disguise two equal display origins.
func applicationURL(value string, exactOrigin bool) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.HasPrefix(value, u.Scheme+"://") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(value, "#") || (exactOrigin && u.Path != "") {
		return nil, fmt.Errorf("expected an HTTP/HTTPS URL without credentials, query or fragment")
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		// Browsers canonicalize shortened, integer, octal and hexadecimal IPv4
		// hosts. Reject these aliases so distinct configured origins stay distinct.
		labels := strings.Split(strings.TrimSuffix(host, "."), ".")
		last := labels[len(labels)-1]
		decimal := last != "" && strings.Trim(last, "0123456789") == ""
		hex := strings.HasPrefix(last, "0x") && strings.Trim(last[2:], "0123456789abcdef") == ""
		if decimal || hex || strings.IndexFunc(host, func(r rune) bool { return r > 127 }) >= 0 {
			return nil, fmt.Errorf("use a canonical IP address or ASCII/punycode hostname")
		}
	}
	if strings.Contains(host, ":") {
		if net.ParseIP(host) == nil {
			return nil, fmt.Errorf("invalid application host")
		}
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid application port")
		}
		if !(u.Scheme == "http" && n == 80 || u.Scheme == "https" && n == 443) {
			host += ":" + strconv.Itoa(n)
		}
	}
	if u.Host != host {
		return nil, fmt.Errorf("use a lowercase application host and omit default ports")
	}
	return u, nil
}
