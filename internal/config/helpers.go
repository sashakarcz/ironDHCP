package config

import "net"

// ParseCIDR is a wrapper around net.ParseCIDR
func ParseCIDR(s string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(s)
}

// ParseMAC is a wrapper around net.ParseMAC
func ParseMAC(s string) (net.HardwareAddr, error) {
	return net.ParseMAC(s)
}

// ParseIP is a wrapper around net.ParseIP
func ParseIP(s string) net.IP {
	return net.ParseIP(s)
}
