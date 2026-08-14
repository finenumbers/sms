import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider, useMutation, useQuery } from "@tanstack/react-query";
import { Navigate, Route, BrowserRouter as Router, Routes } from "react-router-dom";
import { ApiError, Shell } from "ui";
import { api, type AdminMe } from "./api";
import { CallbacksPage } from "./pages/CallbacksPage";
import { ClientDetailPage } from "./pages/ClientDetailPage";
import { ClientsPage } from "./pages/ClientsPage";
import { BillingPage } from "./pages/BillingPage";
import { LoginPage } from "./pages/LoginPage";
import { LogsPage } from "./pages/LogsPage";
import { NumbersPage } from "./pages/NumbersPage";
import { OverviewPage } from "./pages/OverviewPage";
import { SettingsPage } from "./pages/SettingsPage";
import { TariffsPage } from "./pages/TariffsPage";
import { JobsPage } from "./pages/JobsPage";
import { JobDetailPage } from "./pages/JobDetailPage";
import { MonitoringPage } from "./pages/MonitoringPage";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (n, err) => !(err instanceof ApiError && (err.status === 401 || err.status === 403)) && n < 1,
      refetchOnWindowFocus: false,
    },
  },
});

function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => api.get<AdminMe>("/auth/me"),
    retry: false,
  });
}

function Guard({ children }: { children: ReactNode }) {
  const me = useMe();
  if (me.isLoading) {
    return <div className="p-8 text-sm text-zinc-500">Загрузка…</div>;
  }
  if (me.isError) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

function Layout({ children }: { children: ReactNode }) {
  const me = useMe();
  const logout = useMutation({
    mutationFn: () => api.post("/auth/logout"),
    onSuccess: () => {
      queryClient.clear();
      window.location.href = "/login";
    },
  });
  return (
    <Shell
      variant="admin"
      user={me.data?.email}
      onLogout={() => logout.mutate()}
      nav={[
        { to: "/", label: "Обзор" },
        { to: "/clients", label: "Клиенты" },
        { to: "/jobs", label: "Задания" },
        { to: "/billing", label: "Биллинг" },
        { to: "/tariffs", label: "Тарифы" },
        { to: "/monitoring", label: "Мониторинг" },
        { to: "/numbers", label: "Номера" },
        { to: "/settings", label: "Настройки" },
        { to: "/logs", label: "Логи" },
        { to: "/callbacks", label: "Колбэки" },
      ]}
    >
      {children}
    </Shell>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Router>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/*"
            element={
              <Guard>
                <Layout>
                  <Routes>
                    <Route path="/" element={<OverviewPage />} />
                    <Route path="/clients" element={<ClientsPage />} />
                    <Route path="/clients/:id" element={<ClientDetailPage />} />
                    <Route path="/jobs" element={<JobsPage />} />
                    <Route path="/jobs/:id" element={<JobDetailPage />} />
                    <Route path="/billing" element={<BillingPage />} />
                    <Route path="/tariffs" element={<TariffsPage />} />
                    <Route path="/monitoring" element={<MonitoringPage />} />
                    <Route path="/numbers" element={<NumbersPage />} />
                    <Route path="/settings" element={<SettingsPage />} />
                    <Route path="/logs" element={<LogsPage />} />
                    <Route path="/logs/:id" element={<LogsPage />} />
                    <Route path="/callbacks" element={<CallbacksPage />} />
                    <Route path="/callbacks/:id" element={<CallbacksPage />} />
                  </Routes>
                </Layout>
              </Guard>
            }
          />
        </Routes>
      </Router>
    </QueryClientProvider>
  );
}
