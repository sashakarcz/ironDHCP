import type {
  Lease,
  Subnet,
  Reservation,
  GitSyncLog,
  GitStatus,
  DashboardStats,
  HealthResponse,
} from '../types';

const API_BASE = '/api/v1';

class APIClient {
  private authToken: string | null = null;

  setAuthToken(token: string) {
    this.authToken = token;
    localStorage.setItem('auth_token', token);
  }

  clearAuthToken() {
    this.authToken = null;
    localStorage.removeItem('auth_token');
  }

  getAuthToken(): string | null {
    if (!this.authToken) {
      this.authToken = localStorage.getItem('auth_token');
    }
    return this.authToken;
  }

  private async fetch<T>(path: string, options: RequestInit = {}): Promise<T> {
    const token = this.getAuthToken();
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    };

    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers,
    });

    if (response.status === 401) {
      this.clearAuthToken();
      throw new Error('Unauthorized');
    }

    if (!response.ok) {
      throw new Error(`API error: ${response.statusText}`);
    }

    return response.json();
  }

  // Health
  async getHealth(): Promise<HealthResponse> {
    return this.fetch('/health');
  }

  // Dashboard
  async getDashboardStats(): Promise<DashboardStats> {
    return this.fetch('/dashboard/stats');
  }

  // Leases
  async getLeases(params?: { search?: string; state?: string; subnet?: string }): Promise<Lease[]> {
    const query = new URLSearchParams(params as any).toString();
    return this.fetch(`/leases${query ? `?${query}` : ''}`);
  }

  async getLease(id: number): Promise<Lease> {
    return this.fetch(`/leases/${id}`);
  }

  // Subnets
  async getSubnets(): Promise<Subnet[]> {
    return this.fetch('/subnets');
  }

  async getSubnet(network: string): Promise<Subnet> {
    return this.fetch(`/subnets/${encodeURIComponent(network)}`);
  }

  // Reservations
  async getReservations(): Promise<Reservation[]> {
    return this.fetch('/reservations');
  }

  async getReservation(id: number): Promise<Reservation> {
    return this.fetch(`/reservations/${id}`);
  }

  // Git sync
  async getGitStatus(): Promise<GitStatus> {
    return this.fetch('/git/status');
  }

  async getGitLogs(): Promise<{ logs: GitSyncLog[] }> {
    return this.fetch('/git/logs');
  }

  async triggerGitSync(triggeredBy: string = 'manual'): Promise<any> {
    return this.fetch('/git/sync', {
      method: 'POST',
      body: JSON.stringify({ triggered_by: triggeredBy }),
    });
  }

  // Auth
  async login(username: string, password: string): Promise<{ token: string }> {
    return this.fetch('/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });
  }

  // Activity log SSE
  createActivityLogStream(onMessage: (entry: any) => void, onError?: (error: Error) => void) {
    const token = this.getAuthToken();
    const url = new URL(`${API_BASE}/activity/stream`, window.location.origin);

    if (token) {
      url.searchParams.set('token', token);
    }

    const eventSource = new EventSource(url.toString());

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        onMessage(data);
      } catch (error) {
        console.error('Failed to parse SSE message:', error);
      }
    };

    eventSource.onerror = (error) => {
      if (onError) {
        onError(new Error('SSE connection error'));
      }
    };

    return () => eventSource.close();
  }
}

export const api = new APIClient();
