import { useQuery } from "@tanstack/react-query";
import { api, type ClientTariff } from "./api";

export const SMS_PRODUCTS = ["sms_domestic", "sms_international"] as const;
export const LOOKUP_PRODUCTS = ["hlr", "silent_sms"] as const;

export function useClientProducts() {
  const q = useQuery({
    queryKey: ["my-tariff"],
    queryFn: () => api.get<{ items: ClientTariff[] }>("/billing/tariff"),
  });
  const products = new Set((q.data?.items ?? []).map((t) => t.product));
  return {
    products,
    ready: !q.isLoading,
    has: (...need: string[]) => need.some((p) => products.has(p)),
  };
}
