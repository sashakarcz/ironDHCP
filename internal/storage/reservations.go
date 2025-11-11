package storage

import (
	"context"
	"fmt"
	"net"

	"github.com/jackc/pgx/v5"
)

// GetReservationByMAC retrieves a reservation by MAC address
func (s *Store) GetReservationByMAC(ctx context.Context, mac net.HardwareAddr) (*Reservation, error) {
	query := `
		SELECT id, mac::text, ip::text, hostname, subnet::text, description,
		       tftp_server, boot_filename, created_at, updated_at
		FROM reservations
		WHERE mac = $1
	`

	var reservation Reservation
	var macStr, ipStr, subnetStr string
	var tftpServer, bootFilename *string

	err := s.pool.QueryRow(ctx, query, mac.String()).Scan(
		&reservation.ID,
		&macStr,
		&ipStr,
		&reservation.Hostname,
		&subnetStr,
		&reservation.Description,
		&tftpServer,
		&bootFilename,
		&reservation.CreatedAt,
		&reservation.UpdatedAt,
	)

	if tftpServer != nil {
		reservation.TFTPServer = *tftpServer
	}
	if bootFilename != nil {
		reservation.BootFilename = *bootFilename
	}

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get reservation by MAC: %w", err)
	}

	// Parse MAC, IP and subnet
	reservation.MAC, _ = net.ParseMAC(macStr)
	reservation.IP = net.ParseIP(ipStr)
	_, reservation.Subnet, _ = net.ParseCIDR(subnetStr)

	return &reservation, nil
}

// GetReservationByIP retrieves a reservation by IP address
func (s *Store) GetReservationByIP(ctx context.Context, ip net.IP, subnet *net.IPNet) (*Reservation, error) {
	query := `
		SELECT id, mac::text, ip::text, hostname, subnet::text, description,
		       tftp_server, boot_filename, created_at, updated_at
		FROM reservations
		WHERE ip = $1 AND subnet = $2
	`

	var reservation Reservation
	var macStr, ipStr, subnetStr string
	var tftpServer, bootFilename *string

	err := s.pool.QueryRow(ctx, query, ip.String(), subnet.String()).Scan(
		&reservation.ID,
		&macStr,
		&ipStr,
		&reservation.Hostname,
		&subnetStr,
		&reservation.Description,
		&tftpServer,
		&bootFilename,
		&reservation.CreatedAt,
		&reservation.UpdatedAt,
	)

	if tftpServer != nil {
		reservation.TFTPServer = *tftpServer
	}
	if bootFilename != nil {
		reservation.BootFilename = *bootFilename
	}

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get reservation by IP: %w", err)
	}

	// Parse MAC, IP and subnet
	reservation.MAC, _ = net.ParseMAC(macStr)
	reservation.IP = net.ParseIP(ipStr)
	_, reservation.Subnet, _ = net.ParseCIDR(subnetStr)

	return &reservation, nil
}

// CreateReservation creates a new reservation
func (s *Store) CreateReservation(ctx context.Context, reservation *Reservation) error {
	query := `
		INSERT INTO reservations (mac, ip, hostname, subnet, description, tftp_server, boot_filename)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`

	err := s.pool.QueryRow(ctx, query,
		reservation.MAC.String(),
		reservation.IP.String(),
		reservation.Hostname,
		reservation.Subnet.String(),
		reservation.Description,
		reservation.TFTPServer,
		reservation.BootFilename,
	).Scan(&reservation.ID, &reservation.CreatedAt, &reservation.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create reservation: %w", err)
	}

	return nil
}

// UpdateReservation updates an existing reservation
func (s *Store) UpdateReservation(ctx context.Context, reservation *Reservation) error {
	query := `
		UPDATE reservations
		SET ip = $1, hostname = $2, subnet = $3, description = $4, tftp_server = $5, boot_filename = $6
		WHERE id = $7
		RETURNING updated_at
	`

	err := s.pool.QueryRow(ctx, query,
		reservation.IP.String(),
		reservation.Hostname,
		reservation.Subnet.String(),
		reservation.Description,
		reservation.TFTPServer,
		reservation.BootFilename,
		reservation.ID,
	).Scan(&reservation.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update reservation: %w", err)
	}

	return nil
}

// DeleteReservation deletes a reservation
func (s *Store) DeleteReservation(ctx context.Context, id int64) error {
	query := `DELETE FROM reservations WHERE id = $1`

	_, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete reservation: %w", err)
	}

	return nil
}

// DeleteAllReservations removes all reservations (used during config sync)
func (s *Store) DeleteAllReservations(ctx context.Context) error {
	query := `DELETE FROM reservations`

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to delete all reservations: %w", err)
	}

	return nil
}

// GetAllReservations retrieves all reservations
func (s *Store) GetAllReservations(ctx context.Context) ([]*Reservation, error) {
	query := `
		SELECT id, mac::text, host(ip), hostname, subnet::text, description,
		       tftp_server, boot_filename, created_at, updated_at
		FROM reservations
		ORDER BY subnet, ip
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all reservations: %w", err)
	}
	defer rows.Close()

	var reservations []*Reservation
	for rows.Next() {
		var reservation Reservation
		var macStr, ipStr, subnetStr string
		var tftpServer, bootFilename *string

		err := rows.Scan(
			&reservation.ID,
			&macStr,
			&ipStr,
			&reservation.Hostname,
			&subnetStr,
			&reservation.Description,
			&tftpServer,
			&bootFilename,
			&reservation.CreatedAt,
			&reservation.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reservation: %w", err)
		}

		reservation.MAC, _ = net.ParseMAC(macStr)
		reservation.IP = net.ParseIP(ipStr)
		_, reservation.Subnet, _ = net.ParseCIDR(subnetStr)

		if tftpServer != nil {
			reservation.TFTPServer = *tftpServer
		}
		if bootFilename != nil {
			reservation.BootFilename = *bootFilename
		}

		reservations = append(reservations, &reservation)
	}

	return reservations, rows.Err()
}

// GetReservationsBySubnet retrieves all reservations for a specific subnet
func (s *Store) GetReservationsBySubnet(ctx context.Context, subnet *net.IPNet) ([]*Reservation, error) {
	query := `
		SELECT id, mac::text, ip::text, hostname, subnet::text, description,
		       tftp_server, boot_filename, created_at, updated_at
		FROM reservations
		WHERE subnet = $1
		ORDER BY ip
	`

	rows, err := s.pool.Query(ctx, query, subnet.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get reservations by subnet: %w", err)
	}
	defer rows.Close()

	var reservations []*Reservation
	for rows.Next() {
		var reservation Reservation
		var macStr, ipStr, subnetStr string
		var tftpServer, bootFilename *string

		err := rows.Scan(
			&reservation.ID,
			&macStr,
			&ipStr,
			&reservation.Hostname,
			&subnetStr,
			&reservation.Description,
			&tftpServer,
			&bootFilename,
			&reservation.CreatedAt,
			&reservation.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reservation: %w", err)
		}

		reservation.MAC, _ = net.ParseMAC(macStr)
		reservation.IP = net.ParseIP(ipStr)
		_, reservation.Subnet, _ = net.ParseCIDR(subnetStr)

		if tftpServer != nil {
			reservation.TFTPServer = *tftpServer
		}
		if bootFilename != nil {
			reservation.BootFilename = *bootFilename
		}

		reservations = append(reservations, &reservation)
	}

	return reservations, rows.Err()
}
