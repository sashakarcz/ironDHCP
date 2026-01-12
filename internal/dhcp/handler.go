package dhcp

import (
	"context"
	"fmt"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/sashakarcz/irondhcp/internal/events"
	"github.com/sashakarcz/irondhcp/internal/logger"
	"github.com/sashakarcz/irondhcp/internal/storage"
)

// Handler handles DHCP requests
type Handler struct {
	server *Server
	iface  string
}

// Handle processes incoming DHCP requests
func (h *Handler) Handle(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
	ctx := context.Background()

	// Log request with relay information for debugging
	logger.Debug().
		Str("type", req.MessageType().String()).
		Str("mac", req.ClientHWAddr.String()).
		Str("xid", fmt.Sprintf("%x", req.TransactionID)).
		Str("giaddr", req.GatewayIPAddr.String()).
		Str("ciaddr", req.ClientIPAddr.String()).
		Str("peer", peer.String()).
		Str("interface", h.iface).
		Msg("Received DHCP request")

	// Route to appropriate handler based on message type
	var resp *dhcpv4.DHCPv4
	var err error

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		resp, err = h.handleDiscover(ctx, req)
	case dhcpv4.MessageTypeRequest:
		resp, err = h.handleRequest(ctx, req)
	case dhcpv4.MessageTypeRelease:
		err = h.handleRelease(ctx, req)
	case dhcpv4.MessageTypeDecline:
		err = h.handleDecline(ctx, req)
	case dhcpv4.MessageTypeInform:
		resp, err = h.handleInform(ctx, req)
	default:
		logger.Warn().
			Str("type", req.MessageType().String()).
			Msg("Unsupported DHCP message type")
		return
	}

	if err != nil {
		logger.Error().
			Err(err).
			Str("type", req.MessageType().String()).
			Str("mac", req.ClientHWAddr.String()).
			Msg("Failed to handle DHCP request")
		return
	}

	// Send response
	if resp != nil {
		if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
			logger.Error().
				Err(err).
				Str("type", resp.MessageType().String()).
				Msg("Failed to send DHCP response")
		} else {
			logger.Info().
				Str("type", resp.MessageType().String()).
				Str("mac", resp.ClientHWAddr.String()).
				Str("ip", resp.YourIPAddr.String()).
				Msg("Sent DHCP response")
		}
	}
}

// handleDiscover handles DHCPDISCOVER messages
func (h *Handler) handleDiscover(ctx context.Context, req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	// Find subnet for this request
	subnet, err := h.server.findSubnetForRequest(h.iface, req)
	if err != nil {
		return nil, fmt.Errorf("failed to find subnet: %w", err)
	}

	// Extract client identifier
	var clientID string
	if opt := req.Options.Get(dhcpv4.OptionClientIdentifier); opt != nil {
		clientID = string(opt)
	}

	// Extract vendor class identifier
	var vendorClass string
	if opt := req.Options.Get(dhcpv4.OptionClassIdentifier); opt != nil {
		vendorClass = string(opt)
	}

	// Build allocation request
	allocReq := &AllocationRequest{
		MAC:           req.ClientHWAddr,
		Hostname:      req.HostName(),
		Subnet:        subnet.Network,
		Pools:         subnet.Pools,
		LeaseDuration: subnet.LeaseDuration,
		ClientID:      clientID,
		VendorClass:   vendorClass,
	}

	// Allocate IP
	lease, err := h.server.allocator.AllocateIP(ctx, allocReq)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP: %w", err)
	}

	logger.Info().
		Str("mac", req.ClientHWAddr.String()).
		Str("ip", lease.IP.String()).
		Str("subnet", subnet.Network.String()).
		Msg("Allocated IP for DISCOVER")

	// Broadcast DISCOVER event
	if h.server.broadcaster != nil {
		h.server.broadcaster.BroadcastDHCPEvent(
			events.EventTypeDHCPDiscover,
			lease.IP,
			req.ClientHWAddr,
			req.HostName(),
			map[string]interface{}{
				"subnet": subnet.Network.String(),
			},
		)
	}

	// Build OFFER response
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create reply: %w", err)
	}

	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	resp.YourIPAddr = lease.IP
	resp.ServerIPAddr = subnet.Gateway

	// Check for reservation to apply per-host boot options
	reservation, _ := h.server.store.GetReservationByMAC(ctx, req.ClientHWAddr)

	// Add DHCP options (with per-host overrides if reservation exists)
	h.addDHCPOptionsWithReservation(resp, subnet, reservation)

	// Broadcast OFFER event
	if h.server.broadcaster != nil {
		h.server.broadcaster.BroadcastDHCPEvent(
			events.EventTypeDHCPOffer,
			resp.YourIPAddr,
			req.ClientHWAddr,
			req.HostName(),
			map[string]interface{}{
				"subnet": subnet.Network.String(),
			},
		)
	}

	return resp, nil
}

// handleRequest handles DHCPREQUEST messages
func (h *Handler) handleRequest(ctx context.Context, req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	// Find subnet for this request
	subnet, err := h.server.findSubnetForRequest(h.iface, req)
	if err != nil {
		return nil, fmt.Errorf("failed to find subnet: %w", err)
	}

	// Get requested IP
	requestedIP := req.RequestedIPAddress()
	if requestedIP == nil || requestedIP.IsUnspecified() {
		requestedIP = req.ClientIPAddr
	}

	// Verify the requested IP is valid
	if requestedIP.IsUnspecified() {
		return h.sendNAK(req, "No IP address requested")
	}

	// Check if this is a renewal or a new request
	lease, err := h.server.store.GetLeaseByIP(ctx, requestedIP, subnet.Network)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease: %w", err)
	}

	// If lease exists, verify MAC matches
	if lease != nil {
		if lease.MAC.String() != req.ClientHWAddr.String() {
			logger.Warn().
				Str("requested_ip", requestedIP.String()).
				Str("lease_mac", lease.MAC.String()).
				Str("request_mac", req.ClientHWAddr.String()).
				Msg("MAC mismatch for requested IP")
			return h.sendNAK(req, "IP already allocated to another client")
		}

		// Renew the lease
		if err := h.server.allocator.RenewLease(ctx, req.ClientHWAddr, requestedIP, subnet.Network, subnet.LeaseDuration); err != nil {
			return nil, fmt.Errorf("failed to renew lease: %w", err)
		}

		logger.Info().
			Str("mac", req.ClientHWAddr.String()).
			Str("ip", requestedIP.String()).
			Msg("Renewed lease")
	} else {
		// New lease - allocate
		// Extract client identifier
		var clientID string
		if opt := req.Options.Get(dhcpv4.OptionClientIdentifier); opt != nil {
			clientID = string(opt)
		}

		// Extract vendor class identifier
		var vendorClass string
		if opt := req.Options.Get(dhcpv4.OptionClassIdentifier); opt != nil {
			vendorClass = string(opt)
		}

		allocReq := &AllocationRequest{
			MAC:           req.ClientHWAddr,
			Hostname:      req.HostName(),
			Subnet:        subnet.Network,
			Pools:         subnet.Pools,
			LeaseDuration: subnet.LeaseDuration,
			ClientID:      clientID,
			VendorClass:   vendorClass,
		}

		lease, err = h.server.allocator.AllocateIP(ctx, allocReq)
		if err != nil {
			return nil, fmt.Errorf("failed to allocate IP: %w", err)
		}

		// Check if allocated IP matches requested IP
		if !lease.IP.Equal(requestedIP) {
			logger.Warn().
				Str("requested_ip", requestedIP.String()).
				Str("allocated_ip", lease.IP.String()).
				Msg("Allocated IP differs from requested IP")
		}

		logger.Info().
			Str("mac", req.ClientHWAddr.String()).
			Str("ip", lease.IP.String()).
			Msg("Created new lease")
	}

	// Build ACK response
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create reply: %w", err)
	}

	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	resp.YourIPAddr = requestedIP
	resp.ServerIPAddr = subnet.Gateway

	// Check for reservation to apply per-host boot options
	reservation, _ := h.server.store.GetReservationByMAC(ctx, req.ClientHWAddr)

	// Add DHCP options (with per-host overrides if reservation exists)
	h.addDHCPOptionsWithReservation(resp, subnet, reservation)

	// Broadcast REQUEST event
	if h.server.broadcaster != nil {
		h.server.broadcaster.BroadcastDHCPEvent(
			events.EventTypeDHCPRequest,
			resp.YourIPAddr,
			req.ClientHWAddr,
			req.HostName(),
			map[string]interface{}{
				"subnet": subnet.Network.String(),
			},
		)
	}

	// Broadcast ACK event
	if h.server.broadcaster != nil {
		h.server.broadcaster.BroadcastDHCPEvent(
			events.EventTypeDHCPAck,
			resp.YourIPAddr,
			req.ClientHWAddr,
			req.HostName(),
			map[string]interface{}{
				"subnet": subnet.Network.String(),
			},
		)
	}

	return resp, nil
}

// handleRelease handles DHCPRELEASE messages
func (h *Handler) handleRelease(ctx context.Context, req *dhcpv4.DHCPv4) error {
	// Find subnet for this request
	subnet, err := h.server.findSubnetForRequest(h.iface, req)
	if err != nil {
		return fmt.Errorf("failed to find subnet: %w", err)
	}

	// Release the lease
	if err := h.server.allocator.ReleaseLease(ctx, req.ClientIPAddr, subnet.Network); err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}

	logger.Info().
		Str("mac", req.ClientHWAddr.String()).
		Str("ip", req.ClientIPAddr.String()).
		Msg("Released lease")

	// Broadcast RELEASE event
	if h.server.broadcaster != nil {
		h.server.broadcaster.BroadcastDHCPEvent(
			events.EventTypeDHCPRelease,
			req.ClientIPAddr,
			req.ClientHWAddr,
			req.HostName(),
			map[string]interface{}{
				"subnet": subnet.Network.String(),
			},
		)
	}

	return nil
}

// handleDecline handles DHCPDECLINE messages (IP conflict detected)
func (h *Handler) handleDecline(ctx context.Context, req *dhcpv4.DHCPv4) error {
	// Find subnet for this request
	subnet, err := h.server.findSubnetForRequest(h.iface, req)
	if err != nil {
		return fmt.Errorf("failed to find subnet: %w", err)
	}

	requestedIP := req.RequestedIPAddress()
	if requestedIP == nil || requestedIP.IsUnspecified() {
		return fmt.Errorf("no IP address in DECLINE")
	}

	// Mark the lease as declined
	if err := h.server.allocator.DeclineLease(ctx, requestedIP, subnet.Network); err != nil {
		return fmt.Errorf("failed to decline lease: %w", err)
	}

	logger.Warn().
		Str("mac", req.ClientHWAddr.String()).
		Str("ip", requestedIP.String()).
		Msg("Client declined IP (conflict detected)")

	// Broadcast DECLINE event
	if h.server.broadcaster != nil {
		h.server.broadcaster.BroadcastDHCPEvent(
			events.EventTypeDHCPDecline,
			requestedIP,
			req.ClientHWAddr,
			req.HostName(),
			map[string]interface{}{
				"subnet": subnet.Network.String(),
				"reason": "IP conflict detected",
			},
		)
	}

	return nil
}

// handleInform handles DHCPINFORM messages
func (h *Handler) handleInform(ctx context.Context, req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	// Find subnet for this request
	subnet, err := h.server.findSubnetForRequest(h.iface, req)
	if err != nil {
		return nil, fmt.Errorf("failed to find subnet: %w", err)
	}

	// Build ACK response with options only (no IP allocation)
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create reply: %w", err)
	}

	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	resp.YourIPAddr = net.IPv4zero // No IP allocation for INFORM
	resp.ServerIPAddr = subnet.Gateway

	// Add DHCP options
	h.addDHCPOptions(resp, subnet)

	logger.Info().
		Str("mac", req.ClientHWAddr.String()).
		Msg("Responded to INFORM")

	return resp, nil
}

// sendNAK sends a DHCPNAK response
func (h *Handler) sendNAK(req *dhcpv4.DHCPv4, reason string) (*dhcpv4.DHCPv4, error) {
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create NAK: %w", err)
	}

	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
	resp.UpdateOption(dhcpv4.OptMessage(reason))

	logger.Info().
		Str("mac", req.ClientHWAddr.String()).
		Str("reason", reason).
		Msg("Sent NAK")

	// Broadcast NAK event
	requestedIP := req.RequestedIPAddress()
	if requestedIP == nil || requestedIP.IsUnspecified() {
		requestedIP = req.ClientIPAddr
	}
	if h.server.broadcaster != nil {
		h.server.broadcaster.BroadcastDHCPEvent(
			events.EventTypeDHCPNak,
			requestedIP,
			req.ClientHWAddr,
			req.HostName(),
			map[string]interface{}{
				"reason": reason,
			},
		)
	}

	return resp, nil
}

// addDHCPOptions adds standard DHCP options to a response
func (h *Handler) addDHCPOptions(resp *dhcpv4.DHCPv4, subnet *SubnetConfig) {
	h.addDHCPOptionsWithReservation(resp, subnet, nil)
}

// addDHCPOptionsWithReservation adds DHCP options with optional per-host overrides
func (h *Handler) addDHCPOptionsWithReservation(resp *dhcpv4.DHCPv4, subnet *SubnetConfig, reservation *storage.Reservation) {
	// Lease time
	resp.UpdateOption(dhcpv4.OptIPAddressLeaseTime(subnet.LeaseDuration))

	// Router (gateway)
	if subnet.Gateway != nil && !subnet.Gateway.IsUnspecified() {
		resp.UpdateOption(dhcpv4.OptRouter(subnet.Gateway))
	}

	// DNS servers
	if len(subnet.DNSServers) > 0 {
		resp.UpdateOption(dhcpv4.OptDNS(subnet.DNSServers...))
	}

	// Subnet mask
	resp.UpdateOption(dhcpv4.OptSubnetMask(subnet.Network.Mask))

	// Domain name
	if domainName, ok := subnet.Options["domain_name"]; ok {
		resp.UpdateOption(dhcpv4.OptDomainName(domainName))
	}

	// Server identifier
	resp.UpdateOption(dhcpv4.OptServerIdentifier(subnet.Gateway))

	// Boot options (TFTP server and filename)
	// Per-host reservation overrides subnet-level settings
	tftpServer := subnet.TFTPServer
	bootFilename := subnet.BootFilename

	if reservation != nil {
		if reservation.TFTPServer != "" {
			tftpServer = reservation.TFTPServer
		}
		if reservation.BootFilename != "" {
			bootFilename = reservation.BootFilename
		}
	}

	// DHCP option 66: TFTP server name
	if tftpServer != "" {
		resp.UpdateOption(dhcpv4.OptTFTPServerName(tftpServer))
	}

	// DHCP option 67: Bootfile name
	if bootFilename != "" {
		resp.UpdateOption(dhcpv4.OptBootFileName(bootFilename))
	}
}
