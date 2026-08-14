import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { EmptyState, ErrorBox, PAGE_SIZE, PageHeader, Pager, Select, Table, Td, Th, withPage, formatDateTime, formatMoney } from "ui";
import { api, type ClientRow, type LedgerRow } from "../api";

export function BillingPage() {
  const [offset, setOffset] = useState(0);
  const [clientId, setClientId] = useState("");
  const [type, setType] = useState("");
  const clients = useQuery({
    queryKey: ["clients", "billing-filter"],
    queryFn: () => api.get<{ items: ClientRow[] }>(withPage("/clients", 0, { limit: "100" })),
  });
  const params: Record<string, string> = {};
  if (clientId) params.client_id = clientId;
  if (type) params.type = type;
  const ledger = useQuery({
    queryKey: ["platform-ledger", clientId, type, offset],
    queryFn: () => api.get<{ items: LedgerRow[] }>(withPage("/billing/ledger", offset, params)),
  });
  const items = ledger.data?.items ?? [];
  return (
    <div>
      <PageHeader title="Биллинг" />
      <p className="mb-3 text-sm text-zinc-500">Пополнение — в карточке клиента.</p>
      <div className="mb-3 flex flex-wrap gap-2">
        <Select
          value={clientId}
          onChange={(e) => {
            setClientId(e.target.value);
            setOffset(0);
          }}
        >
          <option value="">все клиенты</option>
          {(clients.data?.items ?? []).map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </Select>
        <Select
          value={type}
          onChange={(e) => {
            setType(e.target.value);
            setOffset(0);
          }}
        >
          <option value="">все типы</option>
          {["CREDIT", "HOLD", "DEBIT", "RELEASE", "ADJUSTMENT"].map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </Select>
      </div>
      {ledger.isError ? <ErrorBox error={ledger.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Время</Th>
            <Th>Клиент</Th>
            <Th>Тип</Th>
            <Th>Сумма</Th>
            <Th>Доступно после</Th>
            <Th>Заморожено после</Th>
            <Th>SMS</Th>
            <Th>Комментарий</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((row) => (
            <tr key={row.id}>
              <Td>{formatDateTime(row.created_at)}</Td>
              <Td>
                <Link className="text-blue-700 hover:underline" to={`/clients/${row.client_id}`}>
                  {row.client_name ?? row.client_id}
                </Link>
              </Td>
              <Td>{row.type}</Td>
              <Td>{formatMoney(row.amount, row.currency)}</Td>
              <Td>{formatMoney(row.balance_after_available, row.currency)}</Td>
              <Td>{formatMoney(row.balance_after_held, row.currency)}</Td>
              <Td>{row.sms_message_id ? <code className="text-xs">{row.sms_message_id.slice(0, 8)}</code> : "—"}</Td>
              <Td>{row.description ?? "—"}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!ledger.isLoading && items.length === 0 ? <EmptyState>Операций нет</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
    </div>
  );
}
