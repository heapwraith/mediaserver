// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net"
	"slices"
	"testing"
)

func TestPublicAddrs(t *testing.T) {
	addrs := func(s ...string) []net.Addr {
		out := make([]net.Addr, 0, len(s))
		for _, v := range s {
			_, n, err := net.ParseCIDR(v)
			if err != nil {
				t.Fatal(err)
			}
			ip, _, _ := net.ParseCIDR(v)
			out = append(out, &net.IPNet{IP: ip, Mask: n.Mask})
		}
		return out
	}
	tests := []struct {
		name  string
		host  string
		addrs []net.Addr
		want  []string
	}{
		{"home network only", "0.0.0.0", addrs("192.168.0.10/24", "fe80::1/64", "fd12::5/64", "127.0.0.1/8", "::1/128"), nil},
		{"global IPv6", "0.0.0.0", addrs("192.168.0.10/24", "2804:14d:1::5/64"), []string{"2804:14d:1::5"}},
		{"public IPv4", "::", addrs("203.0.113.7/24"), []string{"203.0.113.7"}},
		{"bound to a private address", "192.168.0.10", addrs("2804:14d:1::5/64"), nil},
		{"bound to a public address", "203.0.113.7", nil, []string{"203.0.113.7"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicAddrs(tt.host, tt.addrs); !slices.Equal(got, tt.want) {
				t.Errorf("publicAddrs = %v, want %v", got, tt.want)
			}
		})
	}
}
