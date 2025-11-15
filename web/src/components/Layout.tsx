import { Link, Outlet, useLocation } from 'react-router-dom';
import { Activity, Database, Network, Settings, GitBranch, LogOut, Radio, Menu, X } from 'lucide-react';
import { api } from '../api/client';
import { useState } from 'react';

export function Layout() {
  const location = useLocation();
  const [sidebarOpen, setSidebarOpen] = useState(false);

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
      {/* Mobile menu button */}
      <div className="fixed top-0 left-0 right-0 z-50 flex items-center justify-between h-16 px-4 bg-gray-900 lg:hidden">
        <h1 className="text-xl font-bold text-white">iron<span className="text-blue-400">DHCP</span></h1>
        <button
          onClick={() => setSidebarOpen(!sidebarOpen)}
          className="p-2 text-gray-300 rounded-lg hover:bg-gray-800"
        >
          {sidebarOpen ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
        </button>
      </div>

      {/* Overlay for mobile */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black bg-opacity-50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <div className={`fixed inset-y-0 left-0 z-40 w-64 bg-gray-800 shadow-lg transform transition-transform duration-300 ease-in-out ${
        sidebarOpen ? 'translate-x-0' : '-translate-x-full'
      } lg:translate-x-0`}>
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
                  onClick={() => setSidebarOpen(false)}
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
      <div className="pt-16 lg:pt-0 lg:pl-64">
        <main className="p-4 sm:p-6 md:p-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
