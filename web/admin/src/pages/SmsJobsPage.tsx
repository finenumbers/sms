import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import {
  Badge,
  EmptyState,
  ErrorBox,
  INFINITE_PAGE_SIZE,
  InfiniteSentinel,
  Input,
  PageHeader,
  Select,
  Table,
  Td,
  Th,
  formatDateTime,
  formatMoney,
  statusTone,
  withPage,
} from "ui";
import { api, type ClientRow, type SmsCampaignRow, type SmsMessageRow, type SmsPage } from "../api";
import { billingStateLabel, campaignInflight, campaignStatusLabel, smsInflight, smsStatusLabel } from "../sms";

const kinds = [
  { id: "campaigns", label: "Рассылки" },
  { id: "outbound", label: "Исходящие" },
  { id: "inbound", label: "Входящие" },
] as const;

type Kind = (typeof kinds)[number]["id"];

function asKind(raw: string | null): Kind {
  if (raw === "outbound" || raw === "inbound") {
    return raw;
  }
  return "campaigns";
}

export function SmsJobsPage() {
  const [search, setSearch] = useSearchParams();
  const kind = asKind(search.get("kind"));
  const clientId = search.get("client_id") ?? "";
  const status = search.get("status") ?? "";
  const source = search.get("source") ?? "";
  const msisdn = search.get("msisdn") ?? "";
  const providerID = search.get("provider_sms_id") ?? "";
  const from = search.get("from") ?? "";

  function setParam(key: string, value: string, drop: string[] = []) {
    const next = new URLSearchParams(search);
    if (value) {
      next.set(key, value);
    } else {
      next.delete(key);
    }
    for (const name of drop) {
      next.delete(name);
    }
    setSearch(next, { replace: true });
  }

  const clients = useQuery({
    queryKey: ["clients", "sms-jobs-filter"],
    queryFn: () => api.get<{ items: ClientRow[] }>(withPage("/clients", 0, {}, 100)),
  });
  const msisdnReady = msisdn === "" || /^[0-9]{8,15}$/.test(msisdn);
  const providerReady = providerID === "" || (!/\s/.test(providerID) && providerID.length <= 128);
  const searchReady = kind === "campaigns" || (msisdnReady && providerReady && !(msisdn && providerID));
  const fromReady = from === "" || /^[0-9]{8,15}$/.test(from);

  const campaigns = useInfiniteQuery({
    queryKey: ["admin-sms-campaigns", clientId, status, from],
    enabled: kind === "campaigns" && fromReady,
    queryFn: ({ pageParam }) =>
      api.get<SmsPage<SmsCampaignRow>>(
        withPage("/sms/campaigns", pageParam, { client_id: clientId, status, from }, INFINITE_PAGE_SIZE),
      ),
    initialPageParam: 0,
    getNextPageParam: (last, _pages, lastParam) => (last.has_more ? lastParam + last.items.length : undefined),
    refetchInterval: (q) =>
      (q.state.data?.pages ?? []).some((p) => p.items.some((row) => campaignInflight(row.status))) ? 4000 : false,
  });
  const messages = useInfiniteQuery({
    queryKey: ["admin-sms-messages", kind, clientId, status, source, msisdn, providerID],
    enabled: kind !== "campaigns" && searchReady,
    queryFn: ({ pageParam }) =>
      api.get<SmsPage<SmsMessageRow>>(
        withPage(
          "/sms/messages",
          pageParam,
          {
            direction: kind,
            client_id: clientId,
            status,
            source: kind === "outbound" ? source : "",
            msisdn,
            provider_sms_id: providerID,
          },
          INFINITE_PAGE_SIZE,
        ),
      ),
    initialPageParam: 0,
    getNextPageParam: (last, _pages, lastParam) => (last.has_more ? lastParam + last.items.length : undefined),
    refetchInterval: (q) =>
      (q.state.data?.pages ?? []).some((p) => p.items.some((row) => smsInflight(row.status))) ? 4000 : false,
  });

  const campaignItems = campaigns.data?.pages.flatMap((p) => p.items) ?? [];
  const messageItems = messages.data?.pages.flatMap((p) => p.items) ?? [];
  const active = kind === "campaigns" ? campaigns : messages;
  const clientOptions = clients.data?.items ?? [];
  const knownClient = clientOptions.some((c) => c.id === clientId);
  const fallbackName =
    campaignItems.find((row) => row.client_id === clientId)?.client_name ||
    messageItems.find((row) => row.client_id === clientId)?.client_name ||
    clientId;

  return (
    <div>
      <PageHeader title="Задачи SMS" />
      <p className="mb-3 text-sm text-zinc-500">Входящие, исходящие и рассылки по всем клиентам. Только просмотр.</p>
      <div className="mb-3 flex flex-wrap gap-2">
        {kinds.map((item) => (
          <button
            key={item.id}
            type="button"
            className={
              kind === item.id
                ? "rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white"
                : "rounded-md border border-zinc-300 px-3 py-1.5 text-sm"
            }
            onClick={() => setParam("kind", item.id === "campaigns" ? "" : item.id, ["status", "source", "msisdn", "provider_sms_id", "from"])}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div className="mb-3 grid gap-3 md:grid-cols-3">
        <Select value={clientId} onChange={(e) => setParam("client_id", e.target.value)}>
          <option value="">все клиенты</option>
          {clientId && !knownClient ? <option value={clientId}>{fallbackName}</option> : null}
          {clientOptions.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </Select>
        {kind === "campaigns" ? (
          <Select value={status} onChange={(e) => setParam("status", e.target.value)}>
            <option value="">все статусы</option>
            {Object.entries(campaignStatusLabel).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
        ) : (
          <Select value={status} onChange={(e) => setParam("status", e.target.value)}>
            <option value="">все статусы</option>
            {Object.entries(smsStatusLabel).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
        )}
        {kind === "campaigns" ? (
          <Input value={from} placeholder="отправитель, точно" onChange={(e) => setParam("from", e.target.value.replace(/\D/g, "").slice(0, 15))} />
        ) : null}
        {kind === "outbound" ? (
          <Select value={source} onChange={(e) => setParam("source", e.target.value)}>
            <option value="">все источники</option>
            <option value="direct">одиночные</option>
            <option value="campaign">из рассылки</option>
          </Select>
        ) : null}
      </div>
      {kind !== "campaigns" ? (
        <div className="mb-3 grid gap-3 md:grid-cols-2">
          <Input
            value={msisdn}
            placeholder="номер, точно"
            onChange={(e) => setParam("msisdn", e.target.value.replace(/\D/g, "").slice(0, 15))}
          />
          <Input
            value={providerID}
            placeholder="id у провайдера"
            onChange={(e) => setParam("provider_sms_id", e.target.value.trim())}
          />
        </div>
      ) : null}
      {!fromReady || !searchReady ? <p className="mb-3 text-sm text-amber-800">Номер — 8–15 цифр. Номер и id провайдера вместе не ищутся.</p> : null}
      {active.isError ? <ErrorBox error={active.error} /> : null}
      {kind === "campaigns" ? <CampaignTable items={campaignItems} /> : <MessageTable items={messageItems} />}
      <InfiniteSentinel disabled={!searchReady || !fromReady || !active.hasNextPage || active.isFetchingNextPage} onVisible={() => void active.fetchNextPage()} />
      {!active.isLoading && !active.isFetching && ((kind === "campaigns" && campaignItems.length === 0) || (kind !== "campaigns" && messageItems.length === 0)) && searchReady && fromReady ? (
        <EmptyState>Задач SMS нет</EmptyState>
      ) : null}
    </div>
  );
}

function CampaignTable({ items }: { items: SmsCampaignRow[] }) {
  return (
    <Table>
      <thead>
        <tr>
          <Th fit>Создано</Th>
          <Th fit>Клиент</Th>
          <Th fit>От</Th>
          <Th fit>Статус</Th>
          <Th fit>Получатели</Th>
          <Th fit>Принято</Th>
          <Th fit>Доставлено</Th>
          <Th fit>Ошибки</Th>
          <Th>Текст</Th>
        </tr>
      </thead>
      <tbody>
        {items.map((row) => (
          <tr key={row.id}>
            <Td fit>
              <Link className="text-blue-700 hover:underline" to={`/sms-jobs/campaigns/${row.id}`}>
                {formatDateTime(row.created_at)}
              </Link>
            </Td>
            <Td fit>
              <Link className="text-blue-700 hover:underline" to={`/clients/${row.client_id}`}>
                {row.client_name || row.client_id}
              </Link>
            </Td>
            <Td fit>{row.from}</Td>
            <Td fit>
              <Badge tone={statusTone(row.status)}>{campaignStatusLabel[row.status] ?? row.status}</Badge>
            </Td>
            <Td fit>{row.total_count}</Td>
            <Td fit>{row.accepted_count}</Td>
            <Td fit>{row.delivered_count}</Td>
            <Td fit className={row.failed_count > 0 ? "text-red-800" : undefined}>
              {row.failed_count}
            </Td>
            <Td>{row.text_preview}</Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

function MessageTable({ items }: { items: SmsMessageRow[] }) {
  return (
    <Table>
      <thead>
        <tr>
          <Th fit>Создано</Th>
          <Th fit>Клиент</Th>
          <Th fit>Источник</Th>
          <Th fit>От</Th>
          <Th fit>Кому</Th>
          <Th fit>Статус</Th>
          <Th fit>Сегменты</Th>
          <Th fit>Сумма</Th>
          <Th fit>Биллинг</Th>
          <Th fit>Провайдер</Th>
          <Th>Текст</Th>
        </tr>
      </thead>
      <tbody>
        {items.map((row) => (
          <tr key={row.id}>
            <Td fit>
              <Link className="text-blue-700 hover:underline" to={`/sms-jobs/messages/${row.id}`}>
                {formatDateTime(row.created_at)}
              </Link>
            </Td>
            <Td fit>
              {row.client_id ? (
                <Link className="text-blue-700 hover:underline" to={`/clients/${row.client_id}`}>
                  {row.client_name || row.client_id}
                </Link>
              ) : (
                "без клиента"
              )}
            </Td>
            <Td fit>{row.campaign_id ? "рассылка" : row.direction === "inbound" ? "входящее" : "одиночное"}</Td>
            <Td fit>{row.from}</Td>
            <Td fit>{row.to}</Td>
            <Td fit>
              <Badge tone={statusTone(row.status)}>{smsStatusLabel[row.status] ?? row.status}</Badge>
            </Td>
            <Td fit>{row.billed_segments ?? "—"}</Td>
            <Td fit>{row.billed_amount ? formatMoney(row.billed_amount, row.currency) : "—"}</Td>
            <Td fit>{billingStateLabel[row.billing_state] ?? row.billing_state}</Td>
            <Td fit>{row.provider_status || "—"}</Td>
            <Td>{row.text_preview}</Td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}
