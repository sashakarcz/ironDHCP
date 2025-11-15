import { useEffect, useState } from 'react';
import { Activity, Database, Server, Wifi, Clock, TrendingUp } from 'lucide-react';
import { api } from '../api/client';
import type { DashboardStats, HealthResponse } from '../types';

export function Dashboard() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchData() {
      try {
        const [statsData, healthData] = await Promise.all([
          api.getDashboardStats(),
          api.getHealth(),
        ]);
        setStats(statsData);
        setHealth(healthData);
      } catch (error) {
        console.error('Failed to fetch dashboard data:', error);
      } finally {
        setLoading(false);
      }
    }

    fetchData();
    const interval = setInterval(fetchData, 10000); // Refresh every 10 seconds

    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-400">Loading...</div>
      </div>
    );
  }

  const statCards = [
    {
      title: 'Active Leases',
      value: stats?.active_leases || 0,
      icon: Activity,
      color: 'bg-green-500',
    },
    {
      title: 'Total Leases',
      value: stats?.total_leases || 0,
      icon: Database,
      color: 'bg-blue-500',
    },
    {
      title: 'Expired Leases',
      value: stats?.expired_leases || 0,
      icon: Clock,
      color: 'bg-yellow-500',
    },
    {
      title: 'Subnets',
      value: stats?.total_subnets || 0,
      icon: Wifi,
      color: 'bg-purple-500',
    },
    {
      title: 'Reservations',
      value: stats?.total_reservations || 0,
      icon: Server,
      color: 'bg-indigo-500',
    },
    {
      title: 'Uptime',
      value: stats?.uptime || 'N/A',
      icon: TrendingUp,
      color: 'bg-pink-500',
    },
  ];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">Dashboard</h1>
        <p className="mt-2 text-gray-400">Overview of your DHCP server</p>
      </div>

      {/* Health Status */}
      {health && (
        <div className={`p-4 rounded-lg ${health.status === 'healthy' ? 'bg-green-900/20 border border-green-500/30' : 'bg-red-900/20 border border-red-500/30'}`}>
          <div className="flex items-center">
            <div className={`w-3 h-3 rounded-full mr-3 ${health.status === 'healthy' ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-white font-medium">
              System Status: <span className="capitalize">{health.status}</span>
            </span>
            <span className="ml-auto text-sm text-gray-400">
              DB Connections: {health.database.connections}/{health.database.max_conns}
            </span>
          </div>
        </div>
      )}

      {/* Stats Grid */}
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
        {statCards.map((card) => (
          <div
            key={card.title}
            className="relative overflow-hidden bg-gray-800 rounded-lg shadow-lg"
          >
            <div className="p-6">
              <div className="flex items-center">
                <div className={`p-3 rounded-lg ${card.color}`}>
                  <card.icon className="w-6 h-6 text-white" />
                </div>
                <div className="ml-4">
                  <p className="text-sm font-medium text-gray-400">{card.title}</p>
                  <p className="text-2xl font-semibold text-white">{card.value}</p>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Quick Info */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="p-6 bg-gray-800 rounded-lg shadow-lg">
          <h2 className="text-lg font-semibold text-white mb-4">Lease Utilization</h2>
          <div className="space-y-3">
            <div>
              <div className="flex justify-between text-sm text-gray-400 mb-1">
                <span>Active</span>
                <span>{stats?.active_leases || 0} / {stats?.total_available_ips || 0}</span>
              </div>
              <div className="w-full bg-gray-700 rounded-full h-2">
                <div
                  className="bg-green-500 h-2 rounded-full transition-all"
                  style={{
                    width: `${stats?.total_available_ips ? (stats.active_leases / stats.total_available_ips) * 100 : 0}%`,
                  }}
                />
              </div>
            </div>
          </div>
        </div>

        <div className="p-6 bg-gray-800 rounded-lg shadow-lg">
          <h2 className="text-lg font-semibold text-white mb-4">System Information</h2>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-400">Version</span>
              <span className="text-white">v1.0.0</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Database Status</span>
              <span className={health?.database.status === 'healthy' ? 'text-green-400' : 'text-red-400'}>
                {health?.database.status}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Uptime</span>
              <span className="text-white">{stats?.uptime || 'N/A'}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
