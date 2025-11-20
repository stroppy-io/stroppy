package ips

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
)

// FirstFreeIP returns the first free IP in cidr that is not in usedIPs.
func FirstFreeIP(ipNet *net.IPNet, usedIPs []string) (net.IP, error) {

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

	return nil, fmt.Errorf("no free IPs in %s", ipNet.String())
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

// RandomIP generates a random IP address within the given CIDR range.
func RandomIP(ipNet *net.IPNet) (net.IP, error) {
	// Calculate the number of available IPs efficiently
	ones, bits := ipNet.Mask.Size()
	if bits == 0 {
		return nil, fmt.Errorf("invalid network mask")
	}

	// Calculate host bits
	hostBits := bits - ones

	// Calculate total number of IPs in the range: 2^hostBits
	totalIPs := new(big.Int).Lsh(big.NewInt(1), uint(hostBits))

	// Exclude network and broadcast addresses (2 IPs)
	// For /31 and /32 (IPv4) or /127 and /128 (IPv6), handle specially
	availableIPs := new(big.Int).Sub(totalIPs, big.NewInt(2))

	if availableIPs.Cmp(big.NewInt(0)) <= 0 {
		return nil, fmt.Errorf("no available IPs in %s", ipNet.String())
	}

	// Generate random offset
	randomOffset, err := rand.Int(rand.Reader, availableIPs)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random number: %w", err)
	}

	// Start from network address
	start := ipNet.IP.Mask(ipNet.Mask)

	// Skip first IP (network address) and add random offset
	offset := new(big.Int).Add(randomOffset, big.NewInt(1))

	// Apply offset to start IP
	ip := addBigIntToIP(cloneIP(start), offset)

	return ip, nil
}

// addBigIntToIP adds a big.Int offset to an IP address
func addBigIntToIP(ip net.IP, offset *big.Int) net.IP {
	// Convert IP to big.Int
	ipInt := new(big.Int).SetBytes(ip)

	// Add offset
	ipInt.Add(ipInt, offset)

	// Convert back to IP
	ipBytes := ipInt.Bytes()

	// Ensure correct length (pad with zeros if needed)
	result := make(net.IP, len(ip))
	copy(result[len(result)-len(ipBytes):], ipBytes)

	return result
}
