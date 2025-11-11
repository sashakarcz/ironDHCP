export interface Lease {
  id: number;
  ip: string;
  mac: string;
  hostname: string;
  subnet: string;
  issued_at: string;
  expires_at: string;
  last_seen: string;
  state: 'active' | 'expired' | 'released' | 'declined';
  client_id: string;
  vendor_class: string;
}

export interface Subnet {
  network: string;
  description: string;
  gateway: string;
  dns_servers: string[];
  lease_duration: string;
  active_leases: number;
  total_ips: number;
  utilization: number;
}

export interface Reservation {
  id: number;
  mac: string;
  ip: string;
  hostname: string;
  subnet: string;
  description: string;
  tftp_server?: string;
  boot_filename?: string;
}

export interface GitSyncLog {
  id: number;
  sync_started_at: string;
  sync_completed_at?: string;
  status: 'success' | 'failed' | 'in_progress';
  commit_hash: string;
  commit_message: string;
  commit_author: string;
  error_message?: string;
  changes_applied?: Record<string, any>;
  triggered_by: 'poll' | 'manual' | 'startup';
  triggered_by_user?: string;
}

export interface GitStatus {
  current_commit: string;
  commit_message: string;
  commit_author: string;
  commit_time: string;
  last_sync_time: string;
  last_sync_status: string;
}

export interface DashboardStats {
  total_leases: number;
  active_leases: number;
  expired_leases: number;
  total_subnets: number;
  total_reservations: number;
  total_available_ips: number;
  uptime: string;
}

export interface ActivityLogEntry {
  timestamp: string;
  type: 'discover' | 'request' | 'offer' | 'ack' | 'nak' | 'release' | 'decline';
  mac: string;
  ip?: string;
  hostname?: string;
  message: string;
}

export interface HealthResponse {
  status: 'healthy' | 'unhealthy';
  database: {
    status: 'healthy' | 'unhealthy';
    connections: number;
    max_conns: number;
  };
  time: string;
}
