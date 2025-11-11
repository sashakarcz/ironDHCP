import { useEffect, useState } from 'react';
import { Search, Filter } from 'lucide-react';
import { api } from '../api/client';
import type { Lease } from '../types';

export function Leases() {
  const [leases, setLeases] = useState<Lease[]>([]);
  const [filteredLeases, setFilteredLeases] = useState<Lease[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [stateFilter, setStateFilter] = useState<string>('all');

  useEffect(() => {
    async function fetchLeases() {
      try {
        const data = await api.getLeases();
        setLeases(data);
        setFilteredLeases(data);
      } catch (error) {
        console.error('Failed to fetch leases:', error);
      } finally {
        setLoading(false);
      }
    }

    fetchLeases();
    const interval = setInterval(fetchLeases, 30000); // Refresh every 30 seconds

    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    let filtered = leases;

    // Apply search filter
    if (search) {
      const searchLower = search.toLowerCase();
      filtered = filtered.filter(
        (lease) =>
          lease.ip.toLowerCase().includes(searchLower) ||
          lease.mac.toLowerCase().includes(searchLower) ||
          lease.hostname.toLowerCase().includes(searchLower)
      );
    }

    // Apply state filter
    if (stateFilter !== 'all') {
      filtered = filtered.filter((lease) => lease.state === stateFilter);
    }

    setFilteredLeases(filtered);
  }, [search, stateFilter, leases]);

  const getStateColor = (state: string) => {
    switch (state) {
      case 'active':
        return 'bg-green-500/10 text-green-400 border-green-500/30';
      case 'expired':
        return 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30';
      case 'released':
        return 'bg-gray-500/10 text-gray-400 border-gray-500/30';
      case 'declined':
        return 'bg-red-500/10 text-red-400 border-red-500/30';
      default:
        return 'bg-blue-500/10 text-blue-400 border-blue-500/30';
    }
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleString();
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-400">Loading leases...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">DHCP Leases</h1>
        <p className="mt-2 text-gray-400">Browse and search all DHCP leases</p>
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        {/* Search */}
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            placeholder="Search by IP, MAC, or hostname..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        {/* State Filter */}
        <div className="flex items-center gap-2">
          <Filter className="w-5 h-5 text-gray-400" />
          <select
            value={stateFilter}
            onChange={(e) => setStateFilter(e.target.value)}
            className="px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="all">All States</option>
            <option value="active">Active</option>
            <option value="expired">Expired</option>
            <option value="released">Released</option>
            <option value="declined">Declined</option>
          </select>
        </div>
      </div>

      {/* Results count */}
      <div className="text-sm text-gray-400">
        Showing {filteredLeases.length} of {leases.length} leases
      </div>

      {/* Leases Table */}
      <div className="overflow-hidden bg-gray-800 rounded-lg shadow-lg">
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-700">
            <thead className="bg-gray-900">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  IP Address
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  MAC Address
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Hostname
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Subnet
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  State
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Expires
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-700">
              {filteredLeases.map((lease) => (
                <tr key={lease.id} className="hover:bg-gray-700/50 transition-colors">
                  <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-white">
                    {lease.ip}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300 font-mono">
                    {lease.mac}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    {lease.hostname || '-'}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    {lease.subnet}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <span className={`inline-flex px-2 py-1 text-xs font-semibold rounded-full border ${getStateColor(lease.state)}`}>
                      {lease.state}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    {formatDate(lease.expires_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {filteredLeases.length === 0 && (
            <div className="text-center py-12">
              <p className="text-gray-400">No leases found</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
