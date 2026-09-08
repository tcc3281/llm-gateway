import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, RequireAuth } from "./auth";
import { ToastProvider } from "./components/Toast";
import { LanguageProvider } from "./i18n";
import Layout from "./components/Layout";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import PeopleAccess from "./pages/PeopleAccess";
import ApiKeys from "./pages/ApiKeys";
import Providers from "./pages/Providers";
import Models from "./pages/Models";
import Alerts from "./pages/Alerts";
import UsageLogs from "./pages/UsageLogs";
import AuditLogs from "./pages/AuditLogs";
import Settings from "./pages/Settings";

export default function App() {
  return (
    <LanguageProvider>
      <AuthProvider>
        <ToastProvider>
          <BrowserRouter>
          <Routes>
          <Route path="/login" element={<Login />} />
          <Route
            path="/"
            element={
              <RequireAuth>
                <Layout />
              </RequireAuth>
            }
          >
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<Dashboard />} />
            <Route path="people" element={<PeopleAccess />} />
            {/* Backwards-compatible aliases */}
            <Route path="users" element={<Navigate to="/people?tab=users" replace />} />
            <Route path="groups" element={<Navigate to="/people?tab=groups" replace />} />
            <Route path="tenants" element={<Navigate to="/people?tab=tenants" replace />} />
            <Route path="analytics/members" element={<Navigate to="/people?tab=analytics" replace />} />
            <Route path="keys" element={<ApiKeys />} />
            <Route path="providers" element={<Providers />} />
            <Route path="models" element={<Models />} />
            <Route path="alerts" element={<Alerts />} />
            <Route path="logs" element={<UsageLogs />} />
            <Route path="audit-logs" element={<AuditLogs />} />
            <Route path="settings" element={<Settings />} />
          </Route>
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
        </BrowserRouter>
        </ToastProvider>
      </AuthProvider>
    </LanguageProvider>
  );
}
