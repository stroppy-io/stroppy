package ips

import (
	"math/big"
	"net"
	"testing"
)

func TestFirstFreeIP(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		usedIPs []string
		want    string
		wantErr bool
	}{
		{
			name:    "first IP in empty range",
			cidr:    "192.168.1.0/24",
			usedIPs: []string{},
			want:    "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "skip used IPs",
			cidr:    "192.168.1.0/24",
			usedIPs: []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
			want:    "192.168.1.4",
			wantErr: false,
		},
		{
			name:    "skip invalid IPs in used list",
			cidr:    "192.168.1.0/24",
			usedIPs: []string{"invalid", "192.168.1.1", "not-an-ip"},
			want:    "192.168.1.2",
			wantErr: false,
		},
		{
			name:    "skip IPs outside CIDR",
			cidr:    "192.168.1.0/24",
			usedIPs: []string{"10.0.0.1", "192.168.2.1"},
			want:    "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "small subnet /30",
			cidr:    "192.168.1.0/30",
			usedIPs: []string{},
			want:    "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "small subnet /30 with used IPs",
			cidr:    "192.168.1.0/30",
			usedIPs: []string{"192.168.1.1"},
			want:    "192.168.1.2",
			wantErr: false,
		},
		{
			name:    "all IPs used in /30",
			cidr:    "192.168.1.0/30",
			usedIPs: []string{"192.168.1.1", "192.168.1.2"},
			want:    "",
			wantErr: true,
		},
		{
			name:    "IPv6 basic",
			cidr:    "2001:db8::/120",
			usedIPs: []string{},
			want:    "2001:db8::1",
			wantErr: false,
		},
		{
			name:    "IPv6 with used IPs",
			cidr:    "2001:db8::/120",
			usedIPs: []string{"2001:db8::1", "2001:db8::2"},
			want:    "2001:db8::3",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR(tt.cidr)
			if err != nil {
				t.Fatalf("failed to parse CIDR: %v", err)
			}

			got, err := FirstFreeIP(ipNet, tt.usedIPs)
			if (err != nil) != tt.wantErr {
				t.Errorf("FirstFreeIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.String() != tt.want {
				t.Errorf("FirstFreeIP() = %v, want %v", got.String(), tt.want)
			}
		})
	}
}

func TestRandomIP(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{
			name:    "IPv4 /24",
			cidr:    "192.168.1.0/24",
			wantErr: false,
		},
		{
			name:    "IPv4 /16",
			cidr:    "10.0.0.0/16",
			wantErr: false,
		},
		{
			name:    "IPv4 /30 (small subnet)",
			cidr:    "192.168.1.0/30",
			wantErr: false,
		},
		{
			name:    "IPv4 /32 (no hosts)",
			cidr:    "192.168.1.1/32",
			wantErr: true,
		},
		{
			name:    "IPv4 /31 (no hosts)",
			cidr:    "192.168.1.0/31",
			wantErr: true,
		},
		{
			name:    "IPv6 /120",
			cidr:    "2001:db8::/120",
			wantErr: false,
		},
		{
			name:    "IPv6 /64",
			cidr:    "2001:db8::/64",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR(tt.cidr)
			if err != nil {
				t.Fatalf("failed to parse CIDR: %v", err)
			}

			got, err := RandomIP(ipNet)
			if (err != nil) != tt.wantErr {
				t.Errorf("RandomIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify IP is within range
				if !ipNet.Contains(got) {
					t.Errorf("RandomIP() = %v is not in network %v", got, ipNet)
				}

				// Verify it's not network or broadcast address
				networkAddr := ipNet.IP.Mask(ipNet.Mask)
				broadcast := lastIP(ipNet)

				if got.Equal(networkAddr) {
					t.Errorf("RandomIP() returned network address %v", got)
				}
				if got.Equal(broadcast) {
					t.Errorf("RandomIP() returned broadcast address %v", got)
				}
			}
		})
	}
}

func TestRandomIP_Distribution(t *testing.T) {
	// Test that RandomIP generates different IPs
	_, ipNet, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed to parse CIDR: %v", err)
	}

	seen := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		ip, err := RandomIP(ipNet)
		if err != nil {
			t.Fatalf("RandomIP() failed: %v", err)
		}
		seen[ip.String()] = true
	}

	// We should see more than one unique IP in 100 iterations
	if len(seen) <= 1 {
		t.Errorf("RandomIP() generated only %d unique IP(s) in %d iterations", len(seen), iterations)
	}
}

func TestIncIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{
			name: "IPv4 simple increment",
			ip:   "192.168.1.1",
			want: "192.168.1.2",
		},
		{
			name: "IPv4 overflow octet",
			ip:   "192.168.1.255",
			want: "192.168.2.0",
		},
		{
			name: "IPv4 overflow multiple octets",
			ip:   "192.168.255.255",
			want: "192.169.0.0",
		},
		{
			name: "IPv6 simple increment",
			ip:   "2001:db8::1",
			want: "2001:db8::2",
		},
		{
			name: "IPv6 overflow",
			ip:   "2001:db8::ffff",
			want: "2001:db8::1:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}

			got := incIP(cloneIP(ip))
			if got.String() != tt.want {
				t.Errorf("incIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLastIP(t *testing.T) {
	tests := []struct {
		name string
		cidr string
		want string
	}{
		{
			name: "IPv4 /24",
			cidr: "192.168.1.0/24",
			want: "192.168.1.255",
		},
		{
			name: "IPv4 /16",
			cidr: "10.0.0.0/16",
			want: "10.0.255.255",
		},
		{
			name: "IPv4 /30",
			cidr: "192.168.1.0/30",
			want: "192.168.1.3",
		},
		{
			name: "IPv4 /32",
			cidr: "192.168.1.1/32",
			want: "192.168.1.1",
		},
		{
			name: "IPv6 /120",
			cidr: "2001:db8::/120",
			want: "2001:db8::ff",
		},
		{
			name: "IPv6 /64",
			cidr: "2001:db8::/64",
			want: "2001:db8::ffff:ffff:ffff:ffff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR(tt.cidr)
			if err != nil {
				t.Fatalf("failed to parse CIDR: %v", err)
			}

			got := lastIP(ipNet)
			if got.String() != tt.want {
				t.Errorf("lastIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCloneIP(t *testing.T) {
	original := net.ParseIP("192.168.1.1")
	clone := cloneIP(original)

	// Verify they are equal
	if !clone.Equal(original) {
		t.Errorf("cloneIP() = %v, want %v", clone, original)
	}

	// Modify clone and verify original is unchanged
	clone[len(clone)-1] = 99
	if original[len(original)-1] == 99 {
		t.Errorf("cloneIP() did not create a true copy - original was modified")
	}
}

func TestNormalizeIPFamily(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		base string
		want string
	}{
		{
			name: "both IPv4",
			ip:   "192.168.1.1",
			base: "10.0.0.1",
			want: "192.168.1.1",
		},
		{
			name: "IPv4-mapped to IPv4 base",
			ip:   "::ffff:192.168.1.1",
			base: "10.0.0.1",
			want: "192.168.1.1",
		},
		{
			name: "both IPv6",
			ip:   "2001:db8::1",
			base: "2001:db8::2",
			want: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			base := net.ParseIP(tt.base)

			got := normalizeIPFamily(ip, base)
			if got.String() != tt.want {
				t.Errorf("normalizeIPFamily() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddBigIntToIP(t *testing.T) {
	tests := []struct {
		name   string
		ip     string
		offset int64
		want   string
	}{
		{
			name:   "IPv4 add 1",
			ip:     "192.168.1.1",
			offset: 1,
			want:   "192.168.1.2",
		},
		{
			name:   "IPv4 add 100",
			ip:     "192.168.1.1",
			offset: 100,
			want:   "192.168.1.101",
		},
		{
			name:   "IPv4 add with overflow",
			ip:     "192.168.1.200",
			offset: 100,
			want:   "192.168.2.44",
		},
		{
			name:   "IPv6 add 1",
			ip:     "2001:db8::1",
			offset: 1,
			want:   "2001:db8::2",
		},
		{
			name:   "IPv6 add large number",
			ip:     "2001:db8::1",
			offset: 1000,
			want:   "2001:db8::3e9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}

			offset := big.NewInt(tt.offset)
			got := addBigIntToIP(ip, offset)

			if got.String() != tt.want {
				t.Errorf("addBigIntToIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkFirstFreeIP(b *testing.B) {
	_, ipNet, _ := net.ParseCIDR("192.168.1.0/24")
	usedIPs := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = FirstFreeIP(ipNet, usedIPs)
	}
}

func BenchmarkRandomIP(b *testing.B) {
	_, ipNet, _ := net.ParseCIDR("192.168.1.0/24")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = RandomIP(ipNet)
	}
}

func BenchmarkRandomIP_LargeNetwork(b *testing.B) {
	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = RandomIP(ipNet)
	}
}
