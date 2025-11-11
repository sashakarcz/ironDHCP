import { useEffect, useState } from 'react';
import { GitBranch, RefreshCw, CheckCircle, XCircle, Clock } from 'lucide-react';
import { api } from '../api/client';
import type { GitStatus, GitSyncLog } from '../types';

export function GitSync() {
  const [status, setStatus] = useState<GitStatus | null>(null);
  const [logs, setLogs] = useState<GitSyncLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);

  const fetchData = async () => {
    try {
      const [statusData, logsData] = await Promise.all([
        api.getGitStatus().catch(() => null),
        api.getGitLogs().catch(() => ({ logs: [] })),
      ]);
      setStatus(statusData);
      setLogs(logsData.logs);
    } catch (error) {
      console.error('Failed to fetch git data:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 15000);
    return () => clearInterval(interval);
  }, []);

  const handleManualSync = async () => {
    setSyncing(true);
    try {
      await api.triggerGitSync('web-ui');
      await fetchData();
    } catch (error) {
      console.error('Manual sync failed:', error);
    } finally {
      setSyncing(false);
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'success':
        return <CheckCircle className="w-5 h-5 text-green-400" />;
      case 'failed':
        return <XCircle className="w-5 h-5 text-red-400" />;
      case 'in_progress':
        return <Clock className="w-5 h-5 text-yellow-400 animate-spin" />;
      default:
        return <Clock className="w-5 h-5 text-gray-400" />;
    }
  };

  const formatDate = (dateString: string) => {
    if (!dateString) return 'Never';
    const date = new Date(dateString);
    return date.toLocaleString();
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-400">Loading Git sync information...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Git Sync</h1>
          <p className="mt-2 text-gray-400">GitOps configuration synchronization</p>
        </div>
        <button
          onClick={handleManualSync}
          disabled={syncing}
          className="flex items-center px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          <RefreshCw className={`w-5 h-5 mr-2 ${syncing ? 'animate-spin' : ''}`} />
          {syncing ? 'Syncing...' : 'Manual Sync'}
        </button>
      </div>

      {/* Current Status */}
      {status && status.current_commit ? (
        <div className="bg-gray-800 rounded-lg shadow-lg p-6">
          <div className="flex items-center mb-4">
            <GitBranch className="w-6 h-6 mr-3 text-blue-400" />
            <h2 className="text-xl font-semibold text-white">Current Status</h2>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-gray-400">Current Commit</p>
              <p className="text-white font-mono text-sm mt-1">{status.current_commit.substring(0, 12)}</p>
            </div>
            <div>
              <p className="text-sm text-gray-400">Commit Author</p>
              <p className="text-white text-sm mt-1">{status.commit_author || '-'}</p>
            </div>
            <div>
              <p className="text-sm text-gray-400">Commit Message</p>
              <p className="text-white text-sm mt-1">{status.commit_message || '-'}</p>
            </div>
            <div>
              <p className="text-sm text-gray-400">Last Sync</p>
              <p className="text-white text-sm mt-1">{formatDate(status.last_sync_time)}</p>
            </div>
          </div>
        </div>
      ) : (
        <div className="bg-gray-800 rounded-lg shadow-lg p-6">
          <p className="text-gray-400 text-center">GitOps is not enabled or no syncs have occurred yet</p>
        </div>
      )}

      {/* Sync History */}
      <div className="bg-gray-800 rounded-lg shadow-lg overflow-hidden">
        <div className="px-6 py-4 bg-gray-900 border-b border-gray-700">
          <h2 className="text-lg font-semibold text-white">Sync History</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-700">
            <thead className="bg-gray-900">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Status
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Commit
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Message
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Started
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Triggered By
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase">
                  Changes
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-700">
              {logs.map((log) => (
                <tr key={log.id} className="hover:bg-gray-700/50">
                  <td className="px-6 py-4 whitespace-nowrap">
                    <div className="flex items-center">
                      {getStatusIcon(log.status)}
                      <span className="ml-2 text-sm text-white capitalize">{log.status}</span>
                    </div>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300 font-mono">
                    {log.commit_hash.substring(0, 12)}
                  </td>
                  <td className="px-6 py-4 text-sm text-gray-300 max-w-md truncate">
                    {log.commit_message || '-'}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    {formatDate(log.sync_started_at)}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    <span className="capitalize">{log.triggered_by}</span>
                    {log.triggered_by_user && ` (${log.triggered_by_user})`}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                    {log.changes_applied ? (
                      <div className="text-xs space-y-1">
                        {Object.entries(log.changes_applied).map(([key, value]) => (
                          <div key={key}>
                            {key}: {String(value)}
                          </div>
                        ))}
                      </div>
                    ) : (
                      '-'
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {logs.length === 0 && (
            <div className="text-center py-12">
              <p className="text-gray-400">No sync history available</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
