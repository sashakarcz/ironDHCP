package storage

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
)

// GetLeaseByMAC retrieves a lease by MAC address
func (s *Store) GetLeaseByMAC(ctx context.Context, mac net.HardwareAddr, subnet *net.IPNet) (*Lease, error) {
	query := `
		SELECT id, ip::text, mac::text, hostname, subnet::text, issued_at, expires_at, last_seen,
		       state, client_id, vendor_class, user_class, allocated_by, created_at, updated_at
		FROM leases
		WHERE mac = $1 AND subnet = $2 AND state = 'active'
		ORDER BY expires_at DESC
		LIMIT 1
	`

	var lease Lease
	var ipStr, macStr, subnetStr string

	err := s.pool.QueryRow(ctx, query, mac.String(), subnet.String()).Scan(
		&lease.ID, &ipStr, &macStr, &lease.Hostname, &subnetStr,
		&lease.IssuedAt, &lease.ExpiresAt, &lease.LastSeen, &lease.State,
		&lease.ClientID, &lease.VendorClass, &lease.UserClass, &lease.AllocatedBy,
		&lease.CreatedAt, &lease.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lease by MAC: %w", err)
	}

	// Parse IP, MAC, and subnet
	lease.IP = net.ParseIP(ipStr)
	lease.MAC, _ = net.ParseMAC(macStr)
	_, lease.Subnet, _ = net.ParseCIDR(subnetStr)

	return &lease, nil
}

// GetLeaseByIP retrieves a lease by IP address
func (s *Store) GetLeaseByIP(ctx context.Context, ip net.IP, subnet *net.IPNet) (*Lease, error) {
	query := `
		SELECT id, ip::text, mac::text, hostname, subnet::text, issued_at, expires_at, last_seen,
		       state, client_id, vendor_class, user_class, allocated_by, created_at, updated_at
		FROM leases
		WHERE ip = $1 AND subnet = $2
		ORDER BY expires_at DESC
		LIMIT 1
	`

	var lease Lease
	var ipStr, macStr, subnetStr string

	err := s.pool.QueryRow(ctx, query, ip.String(), subnet.String()).Scan(
		&lease.ID, &ipStr, &macStr, &lease.Hostname, &subnetStr,
		&lease.IssuedAt, &lease.ExpiresAt, &lease.LastSeen, &lease.State,
		&lease.ClientID, &lease.VendorClass, &lease.UserClass, &lease.AllocatedBy,
		&lease.CreatedAt, &lease.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lease by IP: %w", err)
	}

	// Parse IP, MAC, and subnet
	lease.IP = net.ParseIP(ipStr)
	lease.MAC, _ = net.ParseMAC(macStr)
	_, lease.Subnet, _ = net.ParseCIDR(subnetStr)

	return &lease, nil
}

// CreateLease creates a new lease record
func (s *Store) CreateLease(ctx context.Context, lease *Lease) error {
	query := `
		INSERT INTO leases (ip, mac, hostname, subnet, issued_at, expires_at, last_seen, state, client_id, vendor_class, user_class, allocated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`

	err := s.pool.QueryRow(ctx, query,
		lease.IP.String(),
		lease.MAC.String(),
		lease.Hostname,
		lease.Subnet.String(),
		lease.IssuedAt,
		lease.ExpiresAt,
		lease.LastSeen,
		lease.State,
		lease.ClientID,
		lease.VendorClass,
		lease.UserClass,
		lease.AllocatedBy,
	).Scan(&lease.ID, &lease.CreatedAt, &lease.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}

	return nil
}

// UpdateLease updates an existing lease record
func (s *Store) UpdateLease(ctx context.Context, lease *Lease) error {
	query := `
		UPDATE leases
		SET hostname = $1, issued_at = $2, expires_at = $3, last_seen = $4,
		    state = $5, client_id = $6, vendor_class = $7, user_class = $8, allocated_by = $9
		WHERE id = $10
		RETURNING updated_at
	`

	err := s.pool.QueryRow(ctx, query,
		lease.Hostname,
		lease.IssuedAt,
		lease.ExpiresAt,
		lease.LastSeen,
		lease.State,
		lease.ClientID,
		lease.VendorClass,
		lease.UserClass,
		lease.AllocatedBy,
		lease.ID,
	).Scan(&lease.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update lease: %w", err)
	}

	return nil
}

// RenewLease renews an existing lease with a new expiration time
func (s *Store) RenewLease(ctx context.Context, leaseID int64, expiresAt time.Time) error {
	query := `
		UPDATE leases
		SET expires_at = $1, last_seen = $2, state = 'active'
		WHERE id = $3
	`

	_, err := s.pool.Exec(ctx, query, expiresAt, time.Now(), leaseID)
	if err != nil {
		return fmt.Errorf("failed to renew lease: %w", err)
	}

	return nil
}

// ReleaseLease marks a lease as released
func (s *Store) ReleaseLease(ctx context.Context, ip net.IP, subnet *net.IPNet) error {
	query := `
		UPDATE leases
		SET state = 'released', last_seen = $1
		WHERE ip = $2 AND subnet = $3
	`

	_, err := s.pool.Exec(ctx, query, time.Now(), ip.String(), subnet.String())
	if err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}

	return nil
}

// DeclineLease marks a lease as declined (IP conflict detected)
func (s *Store) DeclineLease(ctx context.Context, ip net.IP, subnet *net.IPNet) error {
	query := `
		UPDATE leases
		SET state = 'declined', last_seen = $1
		WHERE ip = $2 AND subnet = $3
	`

	_, err := s.pool.Exec(ctx, query, time.Now(), ip.String(), subnet.String())
	if err != nil {
		return fmt.Errorf("failed to decline lease: %w", err)
	}

	return nil
}

// GetExpiredLeases returns leases that have expired
func (s *Store) GetExpiredLeases(ctx context.Context, subnet *net.IPNet, rangeStart, rangeEnd net.IP, limit int) ([]*Lease, error) {
	query := `
		SELECT id, ip, mac, hostname, subnet, issued_at, expires_at, last_seen,
		       state, client_id, vendor_class, user_class, allocated_by, created_at, updated_at
		FROM leases
		WHERE subnet = $1
		  AND ip >= $2
		  AND ip <= $3
		  AND state IN ('expired', 'released')
		ORDER BY expires_at ASC
		LIMIT $4
	`

	rows, err := s.pool.Query(ctx, query, subnet.String(), rangeStart.String(), rangeEnd.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get expired leases: %w", err)
	}
	defer rows.Close()

	var leases []*Lease
	for rows.Next() {
		var lease Lease
		var ipStr, subnetStr string

		err := rows.Scan(
			&lease.ID, &ipStr, &lease.MAC, &lease.Hostname, &subnetStr,
			&lease.IssuedAt, &lease.ExpiresAt, &lease.LastSeen, &lease.State,
			&lease.ClientID, &lease.VendorClass, &lease.UserClass, &lease.AllocatedBy,
			&lease.CreatedAt, &lease.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lease: %w", err)
		}

		lease.IP = net.ParseIP(ipStr)
		_, lease.Subnet, _ = net.ParseCIDR(subnetStr)
		leases = append(leases, &lease)
	}

	return leases, rows.Err()
}

// ExpireOldLeases marks leases as expired if they have passed their expiration time
func (s *Store) ExpireOldLeases(ctx context.Context) (int64, error) {
	query := `
		UPDATE leases
		SET state = 'expired'
		WHERE state = 'active' AND expires_at < $1
	`

	result, err := s.pool.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to expire old leases: %w", err)
	}

	return result.RowsAffected(), nil
}

// DeleteOldLeases removes leases older than the specified duration
func (s *Store) DeleteOldLeases(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `
		DELETE FROM leases
		WHERE state IN ('expired', 'released') AND updated_at < $1
	`

	result, err := s.pool.Exec(ctx, query, time.Now().Add(-olderThan))
	if err != nil {
		return 0, fmt.Errorf("failed to delete old leases: %w", err)
	}

	return result.RowsAffected(), nil
}

// GetLeaseStatistics returns aggregated statistics per subnet
func (s *Store) GetLeaseStatistics(ctx context.Context) ([]*LeaseStatistics, error) {
	query := `
		SELECT subnet,
		       COUNT(*) FILTER (WHERE state = 'active') AS active_leases,
		       COUNT(*) FILTER (WHERE state = 'expired') AS expired_leases,
		       COUNT(*) FILTER (WHERE state = 'released') AS released_leases,
		       COUNT(*) FILTER (WHERE state = 'declined') AS declined_leases,
		       MIN(expires_at) FILTER (WHERE state = 'active') AS next_expiry,
		       MAX(last_seen) AS last_activity
		FROM leases
		GROUP BY subnet
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease statistics: %w", err)
	}
	defer rows.Close()

	var stats []*LeaseStatistics
	for rows.Next() {
		var stat LeaseStatistics
		var subnetStr string

		err := rows.Scan(
			&subnetStr,
			&stat.ActiveLeases,
			&stat.ExpiredLeases,
			&stat.ReleasedLeases,
			&stat.DeclinedLeases,
			&stat.NextExpiry,
			&stat.LastActivity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan statistics: %w", err)
		}

		_, stat.Subnet, _ = net.ParseCIDR(subnetStr)
		stats = append(stats, &stat)
	}

	return stats, rows.Err()
}

// GetActiveLeaseCount returns the count of active leases in a subnet
func (s *Store) GetActiveLeaseCount(ctx context.Context, subnet *net.IPNet) (int64, error) {
	query := `
		SELECT COUNT(*)
		FROM leases
		WHERE subnet = $1 AND state = 'active' AND expires_at > $2
	`

	var count int64
	err := s.pool.QueryRow(ctx, query, subnet.String(), time.Now()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get active lease count: %w", err)
	}

	return count, nil
}

// ExpireLeases marks all expired leases as expired
func (s *Store) ExpireLeases(ctx context.Context) (int64, error) {
	query := `
		UPDATE leases
		SET state = 'expired'
		WHERE state = 'active' AND expires_at < $1
	`

	result, err := s.pool.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to expire leases: %w", err)
	}

	return result.RowsAffected(), nil
}

// GetAllLeases retrieves all leases
func (s *Store) GetAllLeases(ctx context.Context) ([]*Lease, error) {
	query := `
		SELECT id, ip, mac, hostname, subnet, issued_at, expires_at, last_seen,
		       state, client_id, vendor_class, user_class, allocated_by, created_at, updated_at
		FROM leases
		ORDER BY expires_at DESC
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all leases: %w", err)
	}
	defer rows.Close()

	var leases []*Lease
	for rows.Next() {
		var lease Lease
		var ipStr, subnetStr string

		err := rows.Scan(
			&lease.ID, &ipStr, &lease.MAC, &lease.Hostname, &subnetStr,
			&lease.IssuedAt, &lease.ExpiresAt, &lease.LastSeen, &lease.State,
			&lease.ClientID, &lease.VendorClass, &lease.UserClass, &lease.AllocatedBy,
			&lease.CreatedAt, &lease.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lease: %w", err)
		}

		lease.IP = net.ParseIP(ipStr)
		_, lease.Subnet, _ = net.ParseCIDR(subnetStr)
		leases = append(leases, &lease)
	}

	return leases, rows.Err()
}
