import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Badge, Button, Card, EmptyState, ErrorBox, Field, PAGE_SIZE, PageHeader, Pager, Select, Table, Td, Textarea, Th, statusTone, withPage } from "ui";
import { api, type Campaign, type NumberOpt } from "../api";

export function CampaignsPage() {
  const nav = useNavigate();
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const numbers = useQuery({ queryKey: ["numbers"], queryFn: () => api.get<{ items: NumberOpt[] }>("/numbers") });
  const list = useQuery({
    queryKey: ["campaigns", offset],
    queryFn: () => api.get<{ items: Campaign[] }>(withPage("/campaigns", offset)),
    refetchInterval: 4000,
  });
  const [from, setFrom] = useState("");
  const [text, setText] = useState("");
  const create = useMutation({
    mutationFn: () => api.post<Campaign>("/campaigns", { from, text }),
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ["campaigns"] });
      nav(`/campaigns/${c.id}`);
    },
  });
  const items = list.data?.items ?? [];

  return (
    <div>
      <PageHeader title="Рассылки SMS" />
      <Card className="mb-4">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <Field label="От">
            <Select value={from} onChange={(e) => setFrom(e.target.value)} required>
              <option value="">выберите номер</option>
              {(numbers.data?.items ?? []).map((n) => (
                <option key={n.id} value={n.msisdn}>
                  {n.msisdn}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Текст (заморожен после старта)">
            <Textarea value={text} onChange={(e) => setText(e.target.value)} rows={3} maxLength={1000} required />
          </Field>
          {create.isError ? <ErrorBox error={create.error} /> : null}
          <Button type="submit" disabled={create.isPending}>
            Создать черновик
          </Button>
        </form>
      </Card>
      {list.isError ? <ErrorBox error={list.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Создана</Th>
            <Th>Откуда</Th>
            <Th>Статус</Th>
            <Th>Всего</Th>
            <Th>Доставлено</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((c) => (
            <tr key={c.id}>
              <Td>
                <Link className="text-blue-700 hover:underline" to={`/campaigns/${c.id}`}>
                  {c.created_at}
                </Link>
              </Td>
              <Td>
                <code>{c.from}</code>
              </Td>
              <Td>
                <Badge tone={statusTone(c.status)}>{c.status}</Badge>
              </Td>
              <Td>{c.total_count}</Td>
              <Td>{c.delivered_count}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>Рассылок нет</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
    </div>
  );
}
