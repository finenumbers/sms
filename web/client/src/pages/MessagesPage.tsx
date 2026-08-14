import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Badge, Button, Card, EmptyState, ErrorBox, Field, Input, PAGE_SIZE, PageHeader, Pager, Select, Table, Td, Textarea, Th, statusTone, withPage, formatMoney } from "ui";
import { api, type Estimate, type Message, type NumberOpt } from "../api";

export function MessagesPage({ inbound = false }: { inbound?: boolean }) {
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const numbers = useQuery({ queryKey: ["numbers"], queryFn: () => api.get<{ items: NumberOpt[] }>("/numbers") });
  const list = useQuery({
    queryKey: ["messages", inbound ? "in" : "all", offset],
    queryFn: () =>
      api.get<{ items: Message[] }>(
        withPage("/messages", offset, inbound ? { direction: "inbound" } : {}),
      ),
    refetchInterval: 4000,
  });
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [text, setText] = useState("");
  const send = useMutation({
    mutationFn: () => api.post<Message>("/messages", { from, to, text }),
    onSuccess: () => {
      setText("");
      setOffset(0);
      void qc.invalidateQueries({ queryKey: ["messages"] });
      void qc.invalidateQueries({ queryKey: ["balance"] });
    },
  });
  const estimate = useQuery({
    queryKey: ["estimate", to, text],
    queryFn: () => api.post<Estimate>("/billing/estimate", { to, text }),
    enabled: to.length >= 11 && text.trim().length > 0,
  });
  const items = list.data?.items ?? [];

  return (
    <div>
      <PageHeader title={inbound ? "Входящие SMS" : "Исходящие SMS"} />
      {inbound ? null : (
        <Card className="mb-4">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              send.mutate();
            }}
            className="grid gap-3 md:grid-cols-2"
          >
            <Field label="От (назначенный DEF)">
              <Select value={from} onChange={(e) => setFrom(e.target.value)} required>
                <option value="">выберите номер</option>
                {(numbers.data?.items ?? []).map((n) => (
                  <option key={n.id} value={n.msisdn}>
                    {n.msisdn}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Кому">
              <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="7XXXXXXXXXX" required />
            </Field>
            <div className="md:col-span-2">
              <Field label="Текст">
                <Textarea value={text} onChange={(e) => setText(e.target.value)} rows={3} maxLength={1000} required />
              </Field>
            </div>
            {estimate.data ? (
              <p className="md:col-span-2 text-xs text-zinc-500">
                {`${estimate.data.segments} PDU × ${formatMoney(estimate.data.unit_sell_price, estimate.data.currency)} = ${formatMoney(estimate.data.total, estimate.data.currency)}`}
              </p>
            ) : null}
            {estimate.isError ? <div className="md:col-span-2"><ErrorBox error={estimate.error} /></div> : null}
            {send.isError ? <div className="md:col-span-2"><ErrorBox error={send.error} /></div> : null}
            <Button type="submit" disabled={send.isPending}>
              Отправить
            </Button>
          </form>
        </Card>
      )}
      {list.isError ? <ErrorBox error={list.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Время</Th>
            <Th>Направление</Th>
            <Th>Откуда</Th>
            <Th>Куда</Th>
            <Th>Статус</Th>
            <Th>Текст</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((m) => (
            <tr key={m.id}>
              <Td>
                <Link className="text-blue-700 hover:underline" to={`/messages/${m.id}`}>
                  {m.created_at}
                </Link>
              </Td>
              <Td>{m.direction === "inbound" ? "вх." : m.direction === "outbound" ? "исх." : m.direction}</Td>
              <Td>
                <code>{m.from}</code>
              </Td>
              <Td>
                <code>{m.to}</code>
              </Td>
              <Td>
                <Badge tone={statusTone(m.status)}>{m.status}</Badge>
              </Td>
              <Td className="max-w-xs truncate">{m.text}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>{inbound ? "Входящих нет" : "Сообщений нет"}</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
    </div>
  );
}
