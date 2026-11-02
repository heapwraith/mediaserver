// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net"
	"net/netip"
)

// publicAddrs returns the addresses of this machine that the internet can
// reach if the router lets it in: global IPv6 (not link-local, not the private
// fc00::/7) and non-private IPv4. host is the listen host: a specific private
// address means nothing is exposed through the others.
func publicAddrs(host string, addrs []net.Addr) []string {
	if ip, err := netip.ParseAddr(host); err == nil && !ip.IsUnspecified() {
		if isPublic(ip) {
			return []string{ip.String()}
		}
		return nil
	}
	var out []string
	for _, a := range addrs {
		p, err := netip.ParsePrefix(a.String())
		if err != nil {
			continue
		}
		if ip := p.Addr().Unmap(); isPublic(ip) {
			out = append(out, ip.String())
		}
	}
	return out
}

func isPublic(ip netip.Addr) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}
