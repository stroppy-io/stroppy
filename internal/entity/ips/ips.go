package ips

import (
	"fmt"
	"net"
)

func FirstFreeYandexIP(cidr string, usedIPs []string) (net.IP, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	// Normalize base
	base := ipNet.IP.Mask(ipNet.Mask)
	bcast := lastIP(ipNet)

	// Yandex: first two usable addresses after network (.1 gateway, .2 DNS) are reserved.
	// So start from base + 3.
	start := cloneIP(base)
	for i := 0; i < 3; i++ {
		incIP(start)
	}

	// Build used set
	used := make(map[string]struct{}, len(usedIPs))
	for _, s := range usedIPs {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		ip = normalizeIPFamily(ip, ipNet.IP)
		if ipNet.Contains(ip) {
			used[ip.String()] = struct{}{}
		}
	}

	for ip := cloneIP(start); lessOrEqual(ip, bcast); incIP(ip) {
		if _, taken := used[ip.String()]; !taken {
			return ip, nil
		}
	}

	return nil, fmt.Errorf("no free Yandex IPs in %s", cidr)
}

// FirstFreeIP returns the first free IP in cidr that is not in usedIPs.
func FirstFreeIP(cidr string, usedIPs []string) (net.IP, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	// Build a set of used IPs (string form).
	used := make(map[string]struct{}, len(usedIPs))
	for _, s := range usedIPs {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		// Normalize to the same family/format as ipNet.IP
		ip = normalizeIPFamily(ip, ipNet.IP)
		if ipNet.Contains(ip) {
			used[ip.String()] = struct{}{}
		}
	}

	// Start from network address.
	start := ipNet.IP.Mask(ipNet.Mask)

	// Compute broadcast (last IP in range).
	broadcast := lastIP(ipNet)

	// Iterate from first host to last host-1.
	for ip := incIP(cloneIP(start)); !ip.Equal(broadcast); ip = incIP(ip) {
		if _, taken := used[ip.String()]; !taken {
			return ip, nil
		}
	}

	return nil, fmt.Errorf("no free IPs in %s", cidr)
}

// incIP increments an IP (IPv4 or IPv6) in-place and also returns it.
func incIP(ip net.IP) net.IP {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
	return ip
}

// lastIP calculates the last IP in the subnet.
func lastIP(ipNet *net.IPNet) net.IP {
	ip := cloneIP(ipNet.IP)
	mask := ipNet.Mask

	for i := 0; i < len(ip); i++ {
		ip[i] |= ^mask[i]
	}
	return ip
}

func cloneIP(ip net.IP) net.IP {
	cp := make(net.IP, len(ip))
	copy(cp, ip)
	return cp
}

// normalizeIPFamily ensures ip has same length/family as base (handles v4-in-v6).
func normalizeIPFamily(ip, base net.IP) net.IP {
	if ip.To4() != nil && base.To4() != nil {
		return ip.To4()
	}
	return ip
}

func lessOrEqual(a, b net.IP) bool {
	for i := range a {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return true
}
