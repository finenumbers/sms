package apikeys

import (
	"fmt"
	"net/netip"
	"strings"
)

func ParseCIDRs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := parseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid cidr %q", ErrValidation, s)
		}
		canon := p.String()
		if _, ok := seen[canon]; ok {
			continue
		}
		seen[canon] = struct{}{}
		out = append(out, canon)
	}
	return out, nil
}

func parseCIDR(s string) (netip.Prefix, error) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked(), nil
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(ip, ip.BitLen()), nil
}

func IPAllowed(cidrs []string, ip *netip.Addr) bool {
	if len(cidrs) == 0 {
		return true
	}
	if ip == nil {
		return false
	}
	addr := ip.Unmap()
	for _, raw := range cidrs {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			continue
		}
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
