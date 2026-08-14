import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Alert, Badge, Button, Card, EmptyState, ErrorBox, Field, Input, PageHeader, Select, Table, Td, Th, statusTone, formatMoney } from "ui";
import { api, probeSMSCConnectivity, type LookupMonitoring, type SMSCBalance, type SMSCConnectivity, type SMSCEstimate, type Settings } from "../api";
import { yn } from "../lookup";

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <div className="text-xs text-zinc-500">{label}</div>
      <div className="mt-1 text-lg font-medium">{value}</div>
    </Card>
  );
}

function flagTone(ok: boolean): "green" | "red" {
  return ok ? "green" : "red";
}

export function MonitoringPage() {
  const [estType, setEstType] = useState("hlr");
  const [estPhone, setEstPhone] = useState("");
  const settings = useQuery({ queryKey: ["settings"], queryFn: () => api.get<Settings>("/settings") });
  const mon = useQuery({
    queryKey: ["lookup-monitoring"],
    queryFn: () => api.get<LookupMonitoring>("/lookups/monitoring"),
    refetchInterval: 15000,
  });
  const balance = useQuery({
    queryKey: ["smsc-balance"],
    queryFn: () => api.get<SMSCBalance>("/provider/smsc/balance"),
    retry: false,
  });
  const connect = useMutation({ mutationFn: () => probeSMSCConnectivity() });
  const estimate = useMutation({
    mutationFn: () =>
      api.post<SMSCEstimate>("/provider/smsc/estimate-cost", {
        type: estType,
        phone: estPhone.trim() || undefined,
      }),
  });

  const probe: SMSCConnectivity | undefined = connect.data;
  const lookupOn = Boolean(settings.data?.lookup_enabled);

  return (
    <div>
      <PageHeader title="Мониторинг" />
      <p className="mb-3 text-sm text-zinc-500">
        Зонды SMSC только читают баланс и считают локальную подпись. Отправка HLR и Silent SMS отсюда не идёт.
      </p>
      {mon.isError ? <ErrorBox error={mon.error} /> : null}
      <div className="mb-4 grid gap-3 md:grid-cols-4">
        <Stat label="Адаптер SMSC" value={yn(mon.data?.adapter_configured)} />
        <Stat label="Секрет колбэка" value={yn(mon.data?.callback_secret_configured)} />
        <Card>
          <div className="text-xs text-zinc-500">HLR / Silent SMS</div>
          <div className="mt-1">
            <Badge tone={lookupOn ? "green" : "amber"}>{lookupOn ? "включено" : "выключено"}</Badge>
          </div>
          <div className="mt-2 text-xs text-zinc-500">флаг и креды SMSC — в Настройках</div>
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Баланс SMSC</div>
          <div className="mt-1 text-lg font-medium">
            {balance.data ? formatMoney(balance.data.balance, balance.data.currency) : "—"}
          </div>
          {balance.isError ? (
            <div className="mt-2">
              <ErrorBox error={balance.error} />
            </div>
          ) : null}
        </Card>
      </div>
      <div className="mb-4 grid gap-3 md:grid-cols-3">
        <Stat label="Запросы SMSC за 24 ч" value={String(mon.data?.requests_24h ?? "—")} />
        <Stat label="Колбэки SMSC за 24 ч" value={String(mon.data?.callbacks_24h ?? "—")} />
        <Stat label="Доставки webhook за 24 ч" value={String(mon.data?.webhooks_24h ?? "—")} />
      </div>
      <div className="mb-4 grid gap-4 md:grid-cols-2">
        <Card>
          <h2 className="mb-3 font-medium">Связь с SMSC</h2>
          <p className="mb-3 text-xs text-zinc-500">balance.php и проверка подписи на нашей стороне. Submit не вызывается.</p>
          <Button type="button" variant="secondary" disabled={connect.isPending} onClick={() => connect.mutate()}>
            Проверить связь
          </Button>
          {probe ? (
            <ul className="mt-3 space-y-1 text-sm">
              <li>
                учётные данные: <Badge tone={flagTone(probe.configured)}>{yn(probe.configured)}</Badge>
              </li>
              <li>
                секрет колбэка:{" "}
                <Badge tone={flagTone(probe.callback_secret_configured)}>{yn(probe.callback_secret_configured)}</Badge>
              </li>
              <li>
                баланс: <Badge tone={flagTone(probe.balance_ok)}>{yn(probe.balance_ok)}</Badge>
                {probe.balance ? ` · ${formatMoney(probe.balance, probe.currency)}` : null}
              </li>
              <li>
                подпись: <Badge tone={flagTone(probe.signature_ok)}>{yn(probe.signature_ok)}</Badge>
              </li>
            </ul>
          ) : null}
          {probe?.balance_error ? <Alert className="mt-3">{probe.balance_error}</Alert> : null}
          {connect.isError ? (
            <div className="mt-3">
              <ErrorBox error={connect.error} />
            </div>
          ) : null}
        </Card>
        <Card>
          <h2 className="mb-3 font-medium">Себестоимость SMSC</h2>
          <p className="mb-3 text-xs text-zinc-500">
            Живой cost=1 у провайдера. Это наша закупка, не цена клиента. Номер можно не указывать.
          </p>
          <Field label="Тип">
            <Select value={estType} onChange={(e) => setEstType(e.target.value)}>
              <option value="hlr">HLR</option>
              <option value="ping">Silent SMS</option>
            </Select>
          </Field>
          <Field label="Телефон (необязательно)">
            <Input value={estPhone} onChange={(e) => setEstPhone(e.target.value)} placeholder="+79000000000" />
          </Field>
          <Button type="button" disabled={estimate.isPending} onClick={() => estimate.mutate()}>
            Узнать себестоимость
          </Button>
          {estimate.data ? (
            <Alert className="mt-3" tone="green">
              {estimate.data.type}: {formatMoney(estimate.data.cost, estimate.data.currency)}
              {estimate.data.phone ? ` · ${estimate.data.phone}` : ""}
            </Alert>
          ) : null}
          {estimate.isError ? (
            <div className="mt-3">
              <ErrorBox error={estimate.error} />
            </div>
          ) : null}
        </Card>
      </div>
      <Card className="mb-4">
        <h2 className="mb-3 font-medium">Последние запросы к SMSC</h2>
        <Table>
          <thead>
            <tr>
              <Th>Время</Th>
              <Th>Вид</Th>
              <Th>Статус</Th>
              <Th>HTTP</Th>
              <Th>Ошибка</Th>
            </tr>
          </thead>
          <tbody>
            {(mon.data?.recent_requests ?? []).map((row) => (
              <tr key={row.id}>
                <Td>{row.created_at}</Td>
                <Td>{row.kind}</Td>
                <Td>
                  <Badge tone={statusTone(row.status)}>{row.status}</Badge>
                </Td>
                <Td>{row.http_status ?? "—"}</Td>
                <Td>{row.error_message ?? row.error_code ?? "—"}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
        {!mon.isLoading && (mon.data?.recent_requests ?? []).length === 0 ? <EmptyState>Запросов нет</EmptyState> : null}
      </Card>
      <Card>
        <h2 className="mb-3 font-medium">Последние колбэки SMSC</h2>
        <Table>
          <thead>
            <tr>
              <Th>Время</Th>
              <Th>Подпись</Th>
              <Th>Обработан</Th>
              <Th>Ошибка</Th>
            </tr>
          </thead>
          <tbody>
            {(mon.data?.recent_callbacks ?? []).map((row) => (
              <tr key={row.id}>
                <Td>{row.created_at}</Td>
                <Td>
                  <Badge tone={row.signature_valid ? "green" : "red"}>{yn(row.signature_valid)}</Badge>
                </Td>
                <Td>{row.processed_at ?? "—"}</Td>
                <Td>{row.process_error ?? "—"}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
        {!mon.isLoading && (mon.data?.recent_callbacks ?? []).length === 0 ? <EmptyState>Колбэков нет</EmptyState> : null}
      </Card>
    </div>
  );
}
