import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider, useMutation, useQuery } from "@tanstack/react-query";
import { Navigate, Route, BrowserRouter as Router, Routes } from "react-router-dom";
import { ApiError, Shell, formatBalance } from "ui";
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
import { RequireProduct } from "./pages/ServiceLocked";
import { WebhooksPage } from "./pages/WebhooksPage";
import { LOOKUP_PRODUCTS, SMS_PRODUCTS, useClientProducts } from "./tariffs";

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
  const { ready, has } = useClientProducts();
  const smsOff = ready && !has(...SMS_PRODUCTS);
  const hlrOff = ready && !has("hlr");
  const silentOff = ready && !has("silent_sms");
  const lookupsOff = ready && !has(...LOOKUP_PRODUCTS);
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
      user={
        me.data ? (
          <div>
            <div className="font-bold text-black">{me.data.client_name}</div>
            <div className="font-normal text-black">{me.data.email}</div>
          </div>
        ) : undefined
      }
      aside={
        bal.data ? (
          <span className="rounded-md bg-[#FBE95F] px-3 py-1.5 text-sm text-black">
            Баланс: <span className="font-bold">{formatBalance(bal.data.available_balance, bal.data.currency)}</span>
          </span>
        ) : null
      }
      onLogout={() => logout.mutate()}
      nav={[
        { to: "/messages", label: "Исходящие SMS", disabled: smsOff },
        { to: "/inbox", label: "Входящие SMS" },
        { to: "/campaigns", label: "Рассылки SMS", disabled: smsOff },
        { separator: true },
        { to: "/hlr", label: "HLR Lookup", disabled: hlrOff },
        { to: "/silent-sms", label: "Silent SMS", disabled: silentOff },
        { to: "/lookups", label: "Проверки HLR / SSMS", disabled: lookupsOff },
        { separator: true },
        { to: "/billing", label: "Биллинг" },
        { to: "/webhooks", label: "Webhooks" },
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
                    <Route
                      path="/messages"
                      element={
                        <RequireProduct anyOf={SMS_PRODUCTS} title="Исходящие SMS">
                          <MessagesPage />
                        </RequireProduct>
                      }
                    />
                    <Route path="/inbox" element={<MessagesPage inbound />} />
                    <Route path="/messages/:id" element={<MessageDetailPage />} />
                    <Route
                      path="/campaigns"
                      element={
                        <RequireProduct anyOf={SMS_PRODUCTS} title="Рассылки SMS">
                          <CampaignsPage />
                        </RequireProduct>
                      }
                    />
                    <Route
                      path="/campaigns/:id"
                      element={
                        <RequireProduct anyOf={SMS_PRODUCTS} title="Рассылки SMS">
                          <CampaignDetailPage />
                        </RequireProduct>
                      }
                    />
                    <Route
                      path="/hlr"
                      element={
                        <RequireProduct anyOf={["hlr"]} title="HLR Lookup">
                          <CheckCreatePage type="hlr" />
                        </RequireProduct>
                      }
                    />
                    <Route
                      path="/silent-sms"
                      element={
                        <RequireProduct anyOf={["silent_sms"]} title="Silent SMS">
                          <CheckCreatePage type="ping" />
                        </RequireProduct>
                      }
                    />
                    <Route
                      path="/lookups"
                      element={
                        <RequireProduct anyOf={LOOKUP_PRODUCTS} title="Проверки HLR / SSMS">
                          <LookupsPage />
                        </RequireProduct>
                      }
                    />
                    <Route
                      path="/lookups/:id"
                      element={
                        <RequireProduct anyOf={LOOKUP_PRODUCTS} title="Проверки HLR / SSMS">
                          <LookupDetailPage />
                        </RequireProduct>
                      }
                    />
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
