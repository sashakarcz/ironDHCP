import { Link, Outlet, useLocation } from 'react-router-dom';
import { Activity, Database, Network, Settings, GitBranch, LogOut, Radio } from 'lucide-react';
import { api } from '../api/client';

export function Layout() {
  const location = useLocation();

  const handleLogout = () => {
    api.clearAuthToken();
    window.location.href = '/login';
  };

  const navItems = [
    { path: '/', icon: Activity, label: 'Dashboard' },
    { path: '/leases', icon: Database, label: 'Leases' },
    { path: '/subnets', icon: Network, label: 'Subnets' },
    { path: '/config', icon: Settings, label: 'Config' },
    { path: '/activity', icon: Radio, label: 'Activity Log' },
    { path: '/git', icon: GitBranch, label: 'Git Sync' },
  ];

  return (
    <div className="min-h-screen bg-gray-900">
      {/* Sidebar */}
      <div className="fixed inset-y-0 left-0 w-64 bg-gray-800 shadow-lg">
        <div className="flex flex-col h-full">
          {/* Logo */}
          <div className="flex items-center justify-center h-16 px-4 bg-gray-900">
            <h1 className="text-2xl font-bold text-white">iron<span className="text-blue-400">DHCP</span></h1>
          </div>

          {/* Navigation */}
          <nav className="flex-1 px-2 py-4 space-y-1 overflow-y-auto">
            {navItems.map(({ path, icon: Icon, label }) => {
              const isActive = location.pathname === path;
              return (
                <Link
                  key={path}
                  to={path}
                  className={`flex items-center px-4 py-3 text-sm font-medium rounded-lg transition-colors ${
                    isActive
                      ? 'bg-gray-900 text-white'
                      : 'text-gray-300 hover:bg-gray-700 hover:text-white'
                  }`}
                >
                  <Icon className="w-5 h-5 mr-3" />
                  {label}
                </Link>
              );
            })}
          </nav>

          {/* Footer */}
          <div className="p-4 border-t border-gray-700">
            <button
              onClick={handleLogout}
              className="flex items-center w-full px-4 py-2 text-sm text-gray-300 transition-colors rounded-lg hover:bg-gray-700 hover:text-white"
            >
              <LogOut className="w-5 h-5 mr-3" />
              Logout
            </button>
          </div>
        </div>
      </div>

      {/* Main content */}
      <div className="pl-64">
        <main className="p-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
