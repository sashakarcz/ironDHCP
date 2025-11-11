import { useEffect, useState } from 'react';
import { FileCode, Server, Database } from 'lucide-react';
import { api } from '../api/client';
import type { Reservation } from '../types';

export function Config() {
  const [reservations, setReservations] = useState<Reservation[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchReservations() {
      try {
        const data = await api.getReservations();
        setReservations(data);
      } catch (error) {
        console.error('Failed to fetch reservations:', error);
      } finally {
        setLoading(false);
      }
    }

    fetchReservations();
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-400">Loading configuration...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">Configuration</h1>
        <p className="mt-2 text-gray-400">View current server configuration</p>
      </div>

      {/* Static Reservations */}
      <div className="bg-gray-800 rounded-lg shadow-lg overflow-hidden">
        <div className="px-6 py-4 bg-gray-900 border-b border-gray-700">
          <div className="flex items-center">
            <Server className="w-5 h-5 mr-3 text-purple-400" />
            <h2 className="text-lg font-semibold text-white">Static Reservations</h2>
            <span className="ml-auto text-sm text-gray-400">{reservations.length} total</span>
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-700">
            <thead className="bg-gray-900">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Hostname
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  MAC Address
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  IP Address
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Subnet
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Boot Options
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-700">
              {reservations.map((reservation) => (
                <tr key={reservation.id} className="hover:bg-gray-700/50">
                  <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-white">
                    {reservation.hostname}
                    {reservation.description && (
                      <div className="text-xs text-gray-400 mt-1">{reservation.description}</div>
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300 font-mono">
                    {reservation.mac}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300 font-mono">
                    {reservation.ip}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    {reservation.subnet}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    {reservation.tftp_server || reservation.boot_filename ? (
                      <div className="space-y-1 text-xs">
                        {reservation.tftp_server && (
                          <div>TFTP: {reservation.tftp_server}</div>
                        )}
                        {reservation.boot_filename && (
                          <div>Boot: {reservation.boot_filename}</div>
                        )}
                      </div>
                    ) : (
                      <span className="text-gray-500">-</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {reservations.length === 0 && (
            <div className="text-center py-12">
              <p className="text-gray-400">No static reservations configured</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
