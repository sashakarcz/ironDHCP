import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { Leases } from './pages/Leases';
import { Subnets } from './pages/Subnets';
import { Config } from './pages/Config';
import { GitSync } from './pages/GitSync';
import Activity from './pages/Activity';
import { Login } from './pages/Login';
import { api } from './api/client';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const token = api.getAuthToken();

  if (!token) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />

        <Route
          path="/"
          element={
            <ProtectedRoute>
              <Layout />
            </ProtectedRoute>
          }
        >
          <Route index element={<Dashboard />} />
          <Route path="leases" element={<Leases />} />
          <Route path="subnets" element={<Subnets />} />
          <Route path="config" element={<Config />} />
          <Route path="activity" element={<Activity />} />
          <Route path="git" element={<GitSync />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
