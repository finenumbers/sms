import { ApiError, createApi } from "ui";

export const api = createApi("/client/v1");

export type ClientMe = {
  id: string;
  email: string;
  role: string;
  client_id: string;
  client_name: string;
};

export type NumberOpt = { id: string; msisdn: string };

export type Message = {
  id: string;
  direction: string;
  from: string;
  to: string;
  text: string;
  status: string;
  provider_sms_id?: string;
  provider_status?: string;
  created_at: string;
  accepted_at?: string;
  sent_at?: string;
  delivered_at?: string;
  failed_at?: string;
  pdu_count?: number;
  billed_segments?: number;
  billed_amount?: string;
  unit_sell_price?: string;
  currency?: string;
  billing_action?: string;
};

export type Balance = {
  available_balance: string;
  held_balance: string;
  currency: string;
};

export type Estimate = {
  billed: boolean;
  product?: string;
  segments: number;
  unit_sell_price?: string;
  total?: string;
  currency?: string;
};

export type BillingStats = Record<string, { spent: string; sms: Record<string, number>; lookups?: Record<string, number> }>;

export type ClientTariff = { product: string; plan_name: string; sell_price: string; currency: string };

export type LookupCheckType = "hlr" | "ping";

export type LookupJob = {
  id: string;
  type: LookupCheckType;
  source: string;
  status: string;
  item_count: number;
  success_count: number;
  failure_count: number;
  unit_sell_price?: string;
  currency?: string;
  estimated_cost?: string;
  actual_cost?: string;
  original_filename?: string;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  work_units?: number;
  deduplicated?: boolean;
  deduplicated_phone_count?: number;
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

export type LookupEstimate = {
  type: LookupCheckType;
  product: string;
  quantity: number;
  unit_sell_price: string;
  estimated_cost: string;
  currency: string;
};

export type LookupPreview = {
  id: string;
  type: LookupCheckType;
  status: string;
  phone_count: number;
  original_filename?: string;
  error_message?: string;
  job_id?: string;
  expires_at: string;
};

export type WebhookEndpoint = {
  id: string;
  url: string;
  description?: string;
  enabled: boolean;
  events: string[];
  consecutive_failures: number;
  created_at: string;
  secret?: string;
};

export type WebhookDelivery = {
  id: string;
  endpoint_id: string;
  event_type: string;
  status: string;
  attempt_count: number;
  last_response_code?: number;
  last_error?: string;
  created_at: string;
  delivered_at?: string;
};

export async function downloadClientFile(path: string, filename: string): Promise<void> {
  const res = await fetch(`/client/v1${path}`, {
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

export type LedgerRow = {
  id: string;
  type: string;
  amount: string;
  currency: string;
  description?: string;
  created_at: string;
};

export type Campaign = {
  id: string;
  from: string;
  text: string;
  status: string;
  total_count: number;
  accepted_count: number;
  delivered_count: number;
  failed_count: number;
  created_at: string;
  updated_at: string;
  recipients?: { total: number; pending: number; enqueued: number; skipped: number; failed: number };
};

export type Recipient = { id: string; to: string; status: string; created_at: string; message_id?: string };

export type APIKey = {
  id: string;
  name: string;
  key_prefix: string;
  status: string;
  created_at: string;
  last_used_at?: string;
};
