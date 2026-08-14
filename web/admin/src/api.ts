import { ApiError, createApi } from "ui";

export const api = createApi("/admin/v1");

export type AdminMe = { id: string; email: string; name: string; role: string };

export type ClientRow = {
  id: string;
  name: string;
  status: string;
  owner_email: string;
  assigned_count: number;
  created_at: string;
  available_balance?: string;
  held_balance?: string;
  currency?: string;
};

export type ClientDetail = {
  id: string;
  name: string;
  status: string;
  created_at: string;
  updated_at: string;
  users: { id: string; email: string; role: string; status: string; created_at: string }[];
  available_balance?: string;
  held_balance?: string;
  currency?: string;
};

export type NumberRow = {
  id: string;
  msisdn: string;
  status: string;
  region?: string | null;
  notes?: string | null;
  supports_sms: boolean;
  client_id?: string;
  client_name?: string;
  assigned_at?: string;
  created_at: string;
};

export type Settings = {
  runexis_email?: string;
  runexis_password_set: boolean;
  dek_key_id?: string;
  callback_base_url?: string;
  sms_directions: { in: boolean; dom_out: boolean; int_out: boolean; in_mass: boolean };
  provider_rps: number;
  client_rps_default: number;
  retention_days: number;
  audit_retention_days: number;
  ops_retention_days: number;
  billing_enforced: boolean;
  low_balance_threshold: string;
  ingress_token_set: boolean;
  ingress_token?: string;
  updated_at: string;
  lookup_enabled?: boolean;
  lookup_check_timeout_sec?: number;
  lookup_poll_interval_sec?: number;
  lookup_max_csv_rows?: number;
  lookup_max_csv_bytes?: number;
  lookup_max_batch_phones?: number;
  lookup_webhook_max_attempts?: number;
  lookup_webhook_timeout_ms?: number;
  lookup_retention_days?: number;
  smsc_base_url?: string;
  smsc_apikey_set?: boolean;
  smsc_callback_secret_set?: boolean;
  smsc_currency?: string;
  smsc_callback_url?: string;
};

export type APIKey = {
  id: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  status: string;
  allowed_cidrs: string[];
  created_at: string;
  last_used_at?: string;
  token?: string;
};

export type CallbackRow = {
  id: string;
  kind: string;
  method: string;
  path: string;
  body_bytes: number;
  created_at: string;
  processed_at?: string | null;
};

export type OpsLogRow = {
  id: string;
  created_at: string;
  category: string;
  level: string;
  action: string;
  request_id?: string;
  actor_type?: string;
  actor_id?: string;
  client_id?: string;
  resource_type?: string;
  resource_id?: string;
  http_method?: string;
  http_path?: string;
  http_status?: number;
  latency_ms?: number;
  summary?: string;
  error?: string;
  ip?: string;
  detail?: unknown;
};

export type LedgerRow = {
  id: string;
  client_id: string;
  client_name?: string;
  type: string;
  amount: string;
  currency: string;
  balance_after_available?: string;
  balance_after_held?: string;
  sms_message_id?: string;
  description?: string;
  created_at: string;
};

export type TariffPlan = {
  id: string;
  code: string;
  name: string;
  product: string;
  sell_price: string;
  currency: string;
  is_default: boolean;
  is_active: boolean;
  description?: string;
};

export type BillingOverview = {
  billing_enforced: boolean;
  low_balance_threshold: string;
  available_total: string;
  held_total: string;
  spent_24h: string;
  spent_7d: string;
  low_balance_clients: number;
  open_holds: number;
  billed_segments_24h: number;
  sms_24h: Record<string, number>;
  sms_7d: Record<string, number>;
  sms_by_product_24h: Record<string, number>;
};

export type ClientBilling = {
  available_balance: string;
  held_balance: string;
  currency: string;
  tariffs: {
    id: string;
    product: string;
    tariff_plan_id: string;
    plan_code: string;
    plan_name: string;
    sell_price: string;
    price_override?: string | null;
    currency: string;
  }[];
  ledger: LedgerRow[];
};

export type LookupCheckType = "hlr" | "ping";

export type LookupJob = {
  id: string;
  client_id: string;
  type: LookupCheckType;
  source: string;
  status: string;
  item_count: number;
  success_count: number;
  failure_count: number;
  unit_sell_price?: string;
  tariff_plan_code?: string;
  currency?: string;
  estimated_cost?: string;
  actual_cost?: string;
  original_filename?: string;
  error_code?: string;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
};

export type LookupItem = {
  id: string;
  job_id: string;
  type: LookupCheckType;
  status: string;
  phone: string;
  result_status?: string;
  is_reachable?: boolean;
  imsi?: string;
  mcc?: string;
  mnc?: string;
  operator_name?: string;
  country_code?: string;
  region?: string;
  msc?: string;
  ported?: boolean;
  roaming?: boolean;
  roaming_country?: string;
  roaming_operator?: string;
  error_code?: string;
  error_message?: string;
  completed_at?: string;
  created_at: string;
};

export type LookupMonitoring = {
  adapter_configured: boolean;
  callback_secret_configured: boolean;
  requests_24h: number;
  callbacks_24h: number;
  webhooks_24h: number;
  recent_requests: LookupRecentRequest[];
  recent_callbacks: LookupRecentCallback[];
};

export type LookupRecentRequest = {
  id: string;
  provider_code: string;
  kind: string;
  status: string;
  provider_message_id?: string | null;
  http_status?: number | null;
  error_code?: string | null;
  error_message?: string | null;
  created_at: string;
};

export type LookupRecentCallback = {
  id: string;
  provider_code: string;
  provider_message_id?: string | null;
  phone?: string | null;
  signature_valid?: boolean | null;
  processed_at?: string | null;
  process_error?: string | null;
  created_at: string;
};

export type SMSCConnectivity = {
  configured: boolean;
  callback_secret_configured: boolean;
  balance_ok: boolean;
  signature_ok: boolean;
  balance?: string;
  currency?: string;
  balance_error?: string;
};

export type SMSCBalance = {
  provider_code: string;
  balance: string;
  currency: string;
};

export type SMSCEstimate = {
  provider_code: string;
  type: string;
  phone: string;
  cost: string;
  currency: string;
  parts?: number;
};

export async function downloadAdminFile(path: string, filename: string): Promise<void> {
  const res = await fetch(`/admin/v1${path}`, {
    credentials: "include",
    headers: { "X-Requested-With": "XMLHttpRequest" },
  });
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: { code?: string; message?: string } };
    throw new ApiError(res.status, data.error?.code ?? "error", data.error?.message ?? res.statusText);
  }
  const blob = await res.blob();
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(href);
}

export async function probeSMSCConnectivity(): Promise<SMSCConnectivity> {
  const res = await fetch("/admin/v1/provider/smsc/connectivity-test", {
    method: "POST",
    credentials: "include",
    headers: { "X-Requested-With": "XMLHttpRequest" },
  });
  const data = (await res.json().catch(() => ({}))) as SMSCConnectivity & {
    error?: { code?: string; message?: string };
  };
  if (typeof data.configured === "boolean") {
    return data;
  }
  throw new ApiError(res.status, data.error?.code ?? "error", data.error?.message ?? res.statusText);
}


