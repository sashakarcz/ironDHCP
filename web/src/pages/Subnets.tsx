import { useEffect, useState } from 'react';
import { Network, Activity } from 'lucide-react';
import { api } from '../api/client';
import type { Subnet } from '../types';

export function Subnets() {
  const [subnets, setSubnets] = useState<Subnet[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchSubnets() {
      try {
        const data = await api.getSubnets();
        setSubnets(data);
      } catch (error) {
        console.error('Failed to fetch subnets:', error);
      } finally {
        setLoading(false);
      }
    }

    fetchSubnets();
    const interval = setInterval(fetchSubnets, 30000);

    return () => clearInterval(interval);
  }, []);

  const getUtilizationColor = (utilization: number) => {
    if (utilization < 50) return 'bg-green-500';
    if (utilization < 80) return 'bg-yellow-500';
    return 'bg-red-500';
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-400">Loading subnets...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">Subnets</h1>
        <p className="mt-2 text-gray-400">Overview of all configured subnets</p>
      </div>

      {/* Subnets Grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {subnets.map((subnet) => (
          <div key={subnet.network} className="bg-gray-800 rounded-lg shadow-lg overflow-hidden">
            {/* Header */}
            <div className="px-6 py-4 bg-gray-900 border-b border-gray-700">
              <div className="flex items-center justify-between">
                <div className="flex items-center">
                  <Network className="w-5 h-5 mr-3 text-blue-400" />
                  <h3 className="text-lg font-semibold text-white">{subnet.network}</h3>
                </div>
                <span className="text-sm text-gray-400">{subnet.description}</span>
              </div>
            </div>

            {/* Body */}
            <div className="p-6 space-y-4">
              {/* Stats Row */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-xs text-gray-400">Active Leases</p>
                  <p className="text-2xl font-semibold text-white">{subnet.active_leases}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-400">Total IPs</p>
                  <p className="text-2xl font-semibold text-white">{subnet.total_ips}</p>
                </div>
              </div>

              {/* Utilization */}
              <div>
                <div className="flex justify-between text-sm mb-2">
                  <span className="text-gray-400">Utilization</span>
                  <span className="text-white font-medium">{subnet.utilization.toFixed(1)}%</span>
                </div>
                <div className="w-full bg-gray-700 rounded-full h-3">
                  <div
                    className={`h-3 rounded-full transition-all ${getUtilizationColor(subnet.utilization)}`}
                    style={{ width: `${subnet.utilization}%` }}
                  />
                </div>
              </div>

              {/* Details */}
              <div className="pt-4 space-y-2 border-t border-gray-700">
                <div className="flex justify-between text-sm">
                  <span className="text-gray-400">Gateway</span>
                  <span className="text-white font-mono">{subnet.gateway}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-gray-400">DNS Servers</span>
                  <span className="text-white font-mono">{subnet.dns_servers.join(', ')}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-gray-400">Lease Duration</span>
                  <span className="text-white">{subnet.lease_duration}</span>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>

      {subnets.length === 0 && (
        <div className="flex flex-col items-center justify-center h-64 bg-gray-800 rounded-lg">
          <Network className="w-16 h-16 text-gray-600 mb-4" />
          <p className="text-gray-400 text-lg">No subnets configured</p>
        </div>
      )}
    </div>
  );
}
