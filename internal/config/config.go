package config

import (
	"fmt"
	"net"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete server configuration
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Observability ObservabilityConfig `yaml:"observability"`
	Git           GitConfig           `yaml:"git"`
	Subnets       []SubnetConfig      `yaml:"subnets"`
}

// ServerConfig holds server-specific settings
type ServerConfig struct {
	Interfaces []InterfaceConfig `yaml:"interfaces"`
	ServerID   string            `yaml:"server_id,omitempty"`
}

// InterfaceConfig specifies which network interfaces to listen on
type InterfaceConfig struct {
	Name string `yaml:"name"`
	IPv4 bool   `yaml:"ipv4"`
	IPv6 bool   `yaml:"ipv6"`
}

// DatabaseConfig holds PostgreSQL connection settings
type DatabaseConfig struct {
	Connection     string        `yaml:"connection"`
	MaxConnections int32         `yaml:"max_connections"`
	MinConnections int32         `yaml:"min_connections"`
	BatchWrites    bool          `yaml:"batch_writes"`
	BatchInterval  time.Duration `yaml:"batch_interval"`
}

// ObservabilityConfig holds monitoring and logging settings
type ObservabilityConfig struct {
	MetricsEnabled bool      `yaml:"metrics_enabled"`
	MetricsPort    int       `yaml:"metrics_port"`
	MetricsPath    string    `yaml:"metrics_path"`
	LogLevel       string    `yaml:"log_level"`
	LogFormat      string    `yaml:"log_format"`
	WebEnabled     bool      `yaml:"web_enabled"`
	WebPort        int       `yaml:"web_port"`
	WebAuth        WebAuth   `yaml:"web_auth"`
}

// WebAuth holds web UI authentication settings
type WebAuth struct {
	Enabled      bool   `yaml:"enabled"`
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

// GitConfig holds GitOps configuration
type GitConfig struct {
	Enabled              bool          `yaml:"enabled"`
	Repository           string        `yaml:"repository,omitempty"`
	Branch               string        `yaml:"branch,omitempty"`
	Auth                 GitAuth       `yaml:"auth,omitempty"`
	PollInterval         time.Duration `yaml:"poll_interval,omitempty"`
	SyncTimeout          time.Duration `yaml:"sync_timeout,omitempty"`
	ValidateBeforeSync   bool          `yaml:"validate_before_sync,omitempty"`
	ConfigPath           string        `yaml:"config_path,omitempty"`
}

// GitAuth holds Git authentication settings
type GitAuth struct {
	Type       string `yaml:"type"` // token, ssh, none
	Token      string `yaml:"token,omitempty"`
	SSHKeyPath string `yaml:"ssh_key_path,omitempty"`
}

// SubnetConfig defines a DHCP subnet
type SubnetConfig struct {
	Network           string              `yaml:"network"`
	Description       string              `yaml:"description"`
	Gateway           string              `yaml:"gateway"`
	DNSServers        []string            `yaml:"dns_servers"`
	LeaseDuration     time.Duration       `yaml:"lease_duration"`
	MaxLeaseDuration  time.Duration       `yaml:"max_lease_duration"`
	Options           map[string]string   `yaml:"options,omitempty"`
	Boot              *BootConfig         `yaml:"boot,omitempty"`
	Pools             []PoolConfig        `yaml:"pools"`
	Reservations      []ReservationConfig `yaml:"reservations,omitempty"`
}

// BootConfig defines PXE/iPXE boot settings
type BootConfig struct {
	TFTPServer string `yaml:"tftp_server,omitempty"` // DHCP option 66
	Filename   string `yaml:"filename,omitempty"`    // DHCP option 67
}

// PoolConfig defines a dynamic IP pool
type PoolConfig struct {
	RangeStart  string `yaml:"range_start"`
	RangeEnd    string `yaml:"range_end"`
	Description string `yaml:"description"`
}

// ReservationConfig defines a static IP reservation
type ReservationConfig struct {
	Hostname    string      `yaml:"hostname"`
	MAC         string      `yaml:"mac"`
	IP          string      `yaml:"ip"`
	Description string      `yaml:"description,omitempty"`
	Boot        *BootConfig `yaml:"boot,omitempty"` // Per-host boot override
}

// Load reads and parses a YAML configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values for optional fields
func (c *Config) setDefaults() {
	// Database defaults
	if c.Database.MaxConnections == 0 {
		c.Database.MaxConnections = 20
	}
	if c.Database.MinConnections == 0 {
		c.Database.MinConnections = 5
	}
	if c.Database.BatchInterval == 0 {
		c.Database.BatchInterval = time.Second
	}

	// Observability defaults
	if c.Observability.MetricsPort == 0 {
		c.Observability.MetricsPort = 9090
	}
	if c.Observability.MetricsPath == "" {
		c.Observability.MetricsPath = "/metrics"
	}
	if c.Observability.LogLevel == "" {
		c.Observability.LogLevel = "info"
	}
	if c.Observability.LogFormat == "" {
		c.Observability.LogFormat = "json"
	}
	if c.Observability.WebPort == 0 {
		c.Observability.WebPort = 8080
	}

	// Git defaults
	if c.Git.Enabled {
		if c.Git.Branch == "" {
			c.Git.Branch = "main"
		}
		if c.Git.PollInterval == 0 {
			c.Git.PollInterval = 60 * time.Second
		}
		if c.Git.SyncTimeout == 0 {
			c.Git.SyncTimeout = 30 * time.Second
		}
		if c.Git.ConfigPath == "" {
			c.Git.ConfigPath = "dhcp.yaml"
		}
	}

	// Subnet defaults
	for i := range c.Subnets {
		if c.Subnets[i].LeaseDuration == 0 {
			c.Subnets[i].LeaseDuration = 24 * time.Hour
		}
		if c.Subnets[i].MaxLeaseDuration == 0 {
			c.Subnets[i].MaxLeaseDuration = 168 * time.Hour // 7 days
		}
	}
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	// Validate server config
	if len(c.Server.Interfaces) == 0 {
		return fmt.Errorf("at least one interface must be configured")
	}

	for i, iface := range c.Server.Interfaces {
		if iface.Name == "" {
			return fmt.Errorf("interface %d: name is required", i)
		}
		if !iface.IPv4 && !iface.IPv6 {
			return fmt.Errorf("interface %s: at least one of ipv4 or ipv6 must be enabled", iface.Name)
		}
		if iface.IPv6 {
			return fmt.Errorf("interface %s: IPv6 is not yet supported", iface.Name)
		}
	}

	if c.Server.ServerID != "" {
		if net.ParseIP(c.Server.ServerID) == nil {
			return fmt.Errorf("server_id must be a valid IP address")
		}
	}

	// Validate database config
	if c.Database.Connection == "" {
		return fmt.Errorf("database connection string is required")
	}
	if c.Database.MaxConnections < c.Database.MinConnections {
		return fmt.Errorf("max_connections must be >= min_connections")
	}

	// Validate observability config
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Observability.LogLevel] {
		return fmt.Errorf("log_level must be one of: debug, info, warn, error")
	}

	validLogFormats := map[string]bool{"json": true, "text": true}
	if !validLogFormats[c.Observability.LogFormat] {
		return fmt.Errorf("log_format must be one of: json, text")
	}

	// Validate Git config
	if c.Git.Enabled {
		if c.Git.Repository == "" {
			return fmt.Errorf("git.repository is required when git is enabled")
		}
		if c.Git.Auth.Type != "" && c.Git.Auth.Type != "token" && c.Git.Auth.Type != "ssh" && c.Git.Auth.Type != "none" {
			return fmt.Errorf("git.auth.type must be one of: token, ssh, none")
		}
	}

	// Validate subnets
	if len(c.Subnets) == 0 {
		return fmt.Errorf("at least one subnet must be configured")
	}

	for i, subnet := range c.Subnets {
		if err := validateSubnet(&subnet, i); err != nil {
			return err
		}
	}

	return nil
}

// validateSubnet validates a single subnet configuration
func validateSubnet(subnet *SubnetConfig, index int) error {
	// Parse network CIDR
	_, network, err := net.ParseCIDR(subnet.Network)
	if err != nil {
		return fmt.Errorf("subnet %d: invalid network CIDR '%s': %w", index, subnet.Network, err)
	}

	// Validate gateway
	gateway := net.ParseIP(subnet.Gateway)
	if gateway == nil {
		return fmt.Errorf("subnet %d: invalid gateway IP '%s'", index, subnet.Gateway)
	}
	if !network.Contains(gateway) {
		return fmt.Errorf("subnet %d: gateway %s is not in network %s", index, subnet.Gateway, subnet.Network)
	}

	// Validate DNS servers
	for j, dnsServer := range subnet.DNSServers {
		if net.ParseIP(dnsServer) == nil {
			return fmt.Errorf("subnet %d: invalid DNS server IP '%s' at index %d", index, dnsServer, j)
		}
	}

	// Validate pools
	if len(subnet.Pools) == 0 {
		return fmt.Errorf("subnet %d: at least one pool must be configured", index)
	}

	for j, pool := range subnet.Pools {
		if err := validatePool(&pool, network, index, j); err != nil {
			return err
		}
	}

	// Validate reservations
	for j, reservation := range subnet.Reservations {
		if err := validateReservation(&reservation, network, index, j); err != nil {
			return err
		}
	}

	return nil
}

// validatePool validates a single pool configuration
func validatePool(pool *PoolConfig, network *net.IPNet, subnetIdx, poolIdx int) error {
	start := net.ParseIP(pool.RangeStart)
	if start == nil {
		return fmt.Errorf("subnet %d, pool %d: invalid range_start IP '%s'", subnetIdx, poolIdx, pool.RangeStart)
	}

	end := net.ParseIP(pool.RangeEnd)
	if end == nil {
		return fmt.Errorf("subnet %d, pool %d: invalid range_end IP '%s'", subnetIdx, poolIdx, pool.RangeEnd)
	}

	// Check if IPs are in the network
	if !network.Contains(start) {
		return fmt.Errorf("subnet %d, pool %d: range_start %s is not in network %s", subnetIdx, poolIdx, pool.RangeStart, network.String())
	}
	if !network.Contains(end) {
		return fmt.Errorf("subnet %d, pool %d: range_end %s is not in network %s", subnetIdx, poolIdx, pool.RangeEnd, network.String())
	}

	// Check if start <= end
	if compareIPs(start, end) > 0 {
		return fmt.Errorf("subnet %d, pool %d: range_start must be <= range_end", subnetIdx, poolIdx)
	}

	return nil
}

// validateReservation validates a single reservation configuration
func validateReservation(reservation *ReservationConfig, network *net.IPNet, subnetIdx, resIdx int) error {
	if reservation.Hostname == "" {
		return fmt.Errorf("subnet %d, reservation %d: hostname is required", subnetIdx, resIdx)
	}

	// Validate MAC address
	if _, err := net.ParseMAC(reservation.MAC); err != nil {
		return fmt.Errorf("subnet %d, reservation %d: invalid MAC address '%s': %w", subnetIdx, resIdx, reservation.MAC, err)
	}

	// Validate IP address
	ip := net.ParseIP(reservation.IP)
	if ip == nil {
		return fmt.Errorf("subnet %d, reservation %d: invalid IP address '%s'", subnetIdx, resIdx, reservation.IP)
	}
	if !network.Contains(ip) {
		return fmt.Errorf("subnet %d, reservation %d: IP %s is not in network %s", subnetIdx, resIdx, reservation.IP, network.String())
	}

	return nil
}

// compareIPs compares two IP addresses, returning -1 if a < b, 0 if a == b, 1 if a > b
func compareIPs(a, b net.IP) int {
	a = a.To4()
	b = b.To4()
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
