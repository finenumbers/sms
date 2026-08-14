import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider, useMutation, useQuery } from "@tanstack/react-query";
import { Navigate, Route, BrowserRouter as Router, Routes } from "react-router-dom";
import { ApiError, Shell, formatMoney } from "ui";
import { api, type Balance, type ClientMe } from "./api";
import { ApiKeysPage } from "./pages/ApiKeysPage";
import { BillingPage } from "./pages/BillingPage";
import { CampaignDetailPage } from "./pages/CampaignDetailPage";
import { CampaignsPage } from "./pages/CampaignsPage";
import { CheckCreatePage } from "./pages/CheckCreatePage";
import { LoginPage } from "./pages/LoginPage";
import { LookupDetailPage } from "./pages/LookupDetailPage";
import { LookupsPage } from "./pages/LookupsPage";
import { MessageDetailPage } from "./pages/MessageDetailPage";
import { MessagesPage } from "./pages/MessagesPage";
import { WebhooksPage } from "./pages/WebhooksPage";

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
    queryFn: () => api.get<ClientMe>("/auth/me"),
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
  const bal = useQuery({
    queryKey: ["balance"],
    queryFn: () => api.get<Balance>("/billing/balance"),
    refetchInterval: 15000,
  });
  const logout = useMutation({
    mutationFn: () => api.post("/auth/logout"),
    onSuccess: () => {
      queryClient.clear();
      window.location.href = "/login";
    },
  });
  return (
    <Shell
      brand="Кабинет Finenumbers"
      user={me.data ? `${me.data.client_name} · ${me.data.email}` : undefined}
      aside={
        bal.data ? (
          <span className="font-medium text-zinc-800">{formatMoney(bal.data.available_balance, bal.data.currency)}</span>
        ) : null
      }
      onLogout={() => logout.mutate()}
      nav={[
        { to: "/messages", label: "Исходящие SMS" },
        { to: "/inbox", label: "Входящие SMS" },
        { to: "/campaigns", label: "Рассылки SMS" },
        { to: "/hlr", label: "Проверка HLR" },
        { to: "/silent-sms", label: "Проверка Silent SMS" },
        { to: "/lookups", label: "Проверки" },
        { to: "/webhooks", label: "Webhooks" },
        { to: "/billing", label: "Биллинг" },
        { to: "/api-keys", label: "API-ключи" },
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
                    <Route path="/" element={<Navigate to="/messages" replace />} />
                    <Route path="/messages" element={<MessagesPage />} />
                    <Route path="/inbox" element={<MessagesPage inbound />} />
                    <Route path="/messages/:id" element={<MessageDetailPage />} />
                    <Route path="/campaigns" element={<CampaignsPage />} />
                    <Route path="/campaigns/:id" element={<CampaignDetailPage />} />
                    <Route path="/hlr" element={<CheckCreatePage type="hlr" />} />
                    <Route path="/silent-sms" element={<CheckCreatePage type="ping" />} />
                    <Route path="/lookups" element={<LookupsPage />} />
                    <Route path="/lookups/:id" element={<LookupDetailPage />} />
                    <Route path="/webhooks" element={<WebhooksPage />} />
                    <Route path="/billing" element={<BillingPage />} />
                    <Route path="/api-keys" element={<ApiKeysPage />} />
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
