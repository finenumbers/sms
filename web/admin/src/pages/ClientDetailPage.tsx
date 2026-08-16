import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Alert, Badge, Button, Card, EmptyState, ErrorBox, Field, Input, PageHeader, Select, Table, Td, Th, statusTone, formatDateTime, formatMoney } from "ui";
import { api, type APIKey, type ClientBilling, type ClientDetail, type TariffPlan } from "../api";
import { priceUnit, productLabel } from "../lookup";

const TARIFF_PRODUCTS = ["sms_domestic", "sms_international", "hlr", "silent_sms"] as const;

export function ClientDetailPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["client", id],
    queryFn: () => api.get<ClientDetail>(`/clients/${id}`),
  });
  const keys = useQuery({
    queryKey: ["apikeys", id],
    queryFn: () => api.get<{ items: APIKey[] }>(`/clients/${id}/api-keys`),
  });
  const billing = useQuery({
    queryKey: ["client-billing", id],
    queryFn: () => api.get<ClientBilling>(`/clients/${id}/billing`),
  });
  const tariffs = useQuery({
    queryKey: ["tariffs"],
    queryFn: () => api.get<{ items: TariffPlan[] }>("/tariffs?limit=100"),
  });
  const [name, setName] = useState<string | null>(null);
  const [keyName, setKeyName] = useState("основной");
  const [cidrs, setCidrs] = useState("");
  const [scopes, setScopes] = useState<string[]>(["sms:send", "sms:read", "campaigns:write"]);
  const [createdToken, setCreatedToken] = useState<string | null>(null);
  const [topupAmount, setTopupAmount] = useState("");
  const [topupComment, setTopupComment] = useState("");
  const [adjustAmount, setAdjustAmount] = useState("");
  const [adjustDir, setAdjustDir] = useState("credit");
  const [adjustComment, setAdjustComment] = useState("");
  const [allowNeg, setAllowNeg] = useState(false);
  const [assignPlans, setAssignPlans] = useState<Record<string, string>>({});
  const [addOpen, setAddOpen] = useState(false);
  const [newUserName, setNewUserName] = useState("");
  const [newUserEmail, setNewUserEmail] = useState("");
  const [newUserPassword, setNewUserPassword] = useState("");
  const [resetUserID, setResetUserID] = useState("");
  const [resetPassword, setResetPassword] = useState("");
  const allScopes = ["sms:send", "sms:read", "campaigns:write", "lookup:write", "lookup:read"] as const;

  const patch = useMutation({
    mutationFn: () => api.patch(`/clients/${id}`, { name: name ?? q.data?.name }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["client", id] }),
  });
  const suspend = useMutation({
    mutationFn: () => api.post(`/clients/${id}/suspend`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["client", id] }),
  });
  const activate = useMutation({
    mutationFn: () => api.post(`/clients/${id}/activate`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["client", id] }),
  });
  const del = useMutation({
    mutationFn: () => api.delete(`/clients/${id}`),
    onSuccess: () => nav("/clients"),
  });
  const createUser = useMutation({
    mutationFn: () => api.post(`/clients/${id}/users`, { name: newUserName, email: newUserEmail, password: newUserPassword }),
    onSuccess: () => {
      setAddOpen(false);
      setNewUserName("");
      setNewUserEmail("");
      setNewUserPassword("");
      void qc.invalidateQueries({ queryKey: ["client", id] });
    },
  });
  const resetUser = useMutation({
    mutationFn: ({ userID, password }: { userID: string; password: string }) =>
      api.post(`/clients/${id}/users/${userID}/password`, { password }),
    onSuccess: () => {
      setResetUserID("");
      setResetPassword("");
    },
  });
  const disableUser = useMutation({
    mutationFn: (userID: string) => api.post(`/clients/${id}/users/${userID}/disable`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["client", id] }),
  });
  const enableUser = useMutation({
    mutationFn: (userID: string) => api.post(`/clients/${id}/users/${userID}/enable`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["client", id] }),
  });
  const createKey = useMutation({
    mutationFn: () =>
      api.post<APIKey>(`/clients/${id}/api-keys`, {
        name: keyName,
        scopes,
        allowed_cidrs: cidrs
          .split(/[\s,]+/)
          .map((s) => s.trim())
          .filter(Boolean),
      }),
    onSuccess: (row) => {
      setCreatedToken(row.token ?? null);
      void qc.invalidateQueries({ queryKey: ["apikeys", id] });
    },
  });
  const revoke = useMutation({
    mutationFn: (keyID: string) => api.post(`/clients/${id}/api-keys/${keyID}/revoke`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["apikeys", id] }),
  });
  const topup = useMutation({
    mutationFn: () =>
      api.post(`/clients/${id}/billing/topup`, {
        amount: topupAmount,
        comment: topupComment,
        idempotency_key: crypto.randomUUID(),
      }),
    onSuccess: () => {
      setTopupAmount("");
      setTopupComment("");
      void qc.invalidateQueries({ queryKey: ["client-billing", id] });
      void qc.invalidateQueries({ queryKey: ["client", id] });
      void qc.invalidateQueries({ queryKey: ["clients"] });
    },
  });
  const adjust = useMutation({
    mutationFn: () =>
      api.post(`/clients/${id}/billing/adjust`, {
        amount: adjustAmount,
        direction: adjustDir,
        comment: adjustComment,
        allow_negative: allowNeg,
        idempotency_key: crypto.randomUUID(),
      }),
    onSuccess: () => {
      setAdjustAmount("");
      setAdjustComment("");
      void qc.invalidateQueries({ queryKey: ["client-billing", id] });
      void qc.invalidateQueries({ queryKey: ["client", id] });
    },
  });
  const assign = useMutation({
    mutationFn: (arg: { product: string; tariff_plan_id: string }) => api.post(`/clients/${id}/tariff`, arg),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["client-billing", id] }),
  });
  const unassign = useMutation({
    mutationFn: (product: string) => api.delete(`/clients/${id}/tariff/${product}`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["client-billing", id] }),
  });

  if (q.isError) {
    return <ErrorBox error={q.error} />;
  }
  if (!q.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const c = q.data;
  const assigned = Object.fromEntries((billing.data?.tariffs ?? []).map((t) => [t.product, t]));
  const activeOwners = c.users.filter((u) => u.status === "active").length;

  return (
    <div>
      <PageHeader
        title={c.name}
        actions={
          <Link className="text-sm text-blue-700 hover:underline" to="/clients">
            ← к списку
          </Link>
        }
      />
      <div className="mb-4">
        <Badge tone={statusTone(c.status)}>{c.status}</Badge>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <h2 className="mb-3 font-medium">Карточка</h2>
          <Field label="Название">
            <Input value={name ?? c.name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Button type="button" onClick={() => patch.mutate()} disabled={patch.isPending}>
            Сохранить
          </Button>
          {patch.isError ? <div className="mt-2"><ErrorBox error={patch.error} /></div> : null}
          <div className="mt-4 flex flex-wrap gap-2">
            {c.status === "active" ? (
              <Button variant="secondary" type="button" onClick={() => suspend.mutate()}>
                Приостановить
              </Button>
            ) : null}
            {c.status === "suspended" ? (
              <Button variant="secondary" type="button" onClick={() => activate.mutate()}>
                Активировать
              </Button>
            ) : null}
            <Button
              variant="danger"
              type="button"
              onClick={() => {
                if (window.confirm("Удалить клиента? Номера снимутся, ключи отзовутся.")) {
                  del.mutate();
                }
              }}
            >
              Удалить
            </Button>
          </div>
          {del.isError ? <div className="mt-2"><ErrorBox error={del.error} /></div> : null}
        </Card>
        <Card>
          <div className="mb-3 flex items-center justify-between gap-2">
            <h2 className="font-medium">Участники</h2>
            <Button type="button" onClick={() => setAddOpen((v) => !v)}>
              Добавить пользователя
            </Button>
          </div>
          {addOpen ? (
            <div className="mb-4 rounded-lg border border-zinc-200 p-3">
              <Field label="ФИО">
                <Input value={newUserName} onChange={(e) => setNewUserName(e.target.value)} />
              </Field>
              <Field label="Email">
                <Input type="email" value={newUserEmail} onChange={(e) => setNewUserEmail(e.target.value)} />
              </Field>
              <Field label="Пароль (мин. 10)">
                <Input type="password" value={newUserPassword} onChange={(e) => setNewUserPassword(e.target.value)} minLength={10} />
              </Field>
              {createUser.isError ? <ErrorBox error={createUser.error} /> : null}
              <Button
                type="button"
                disabled={createUser.isPending || newUserName.trim() === "" || newUserEmail.trim() === "" || newUserPassword.length < 10}
                onClick={() => createUser.mutate()}
              >
                Сохранить
              </Button>
            </div>
          ) : null}
          <ul className="space-y-3">
            {c.users.map((u) => (
              <li key={u.id} className="rounded-lg border border-zinc-200 p-3">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div>
                    <div className="font-medium">{u.email}</div>
                    <div className="text-sm text-zinc-500">
                      {(u.name && u.name.trim()) || "—"} · {u.role.toUpperCase()}
                    </div>
                  </div>
                  <Badge tone={u.status === "active" ? "green" : "zinc"}>{u.status.toUpperCase()}</Badge>
                </div>
                <div className="mt-2 flex flex-wrap gap-2">
                  {u.status === "active" ? (
                    <Button
                      variant="secondary"
                      type="button"
                      disabled={disableUser.isPending || activeOwners <= 1}
                      onClick={() => {
                        if (window.confirm(`Отключить ${u.email}?`)) {
                          disableUser.mutate(u.id);
                        }
                      }}
                    >
                      Отключить
                    </Button>
                  ) : (
                    <Button variant="secondary" type="button" disabled={enableUser.isPending} onClick={() => enableUser.mutate(u.id)}>
                      Включить
                    </Button>
                  )}
                  <Button variant="ghost" type="button" onClick={() => setResetUserID(resetUserID === u.id ? "" : u.id)}>
                    Сбросить пароль
                  </Button>
                </div>
                {resetUserID === u.id ? (
                  <div className="mt-2">
                    <Field label="Новый пароль (мин. 10)">
                      <Input type="password" value={resetPassword} onChange={(e) => setResetPassword(e.target.value)} minLength={10} />
                    </Field>
                    {resetUser.isError ? <ErrorBox error={resetUser.error} /> : null}
                    {resetUser.isSuccess ? <Alert tone="green">Пароль обновлён, сессии этого пользователя отозваны</Alert> : null}
                    <Button
                      type="button"
                      disabled={resetPassword.length < 10 || resetUser.isPending}
                      onClick={() => resetUser.mutate({ userID: u.id, password: resetPassword })}
                    >
                      Сохранить пароль
                    </Button>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
          {disableUser.isError ? <div className="mt-2"><ErrorBox error={disableUser.error} /></div> : null}
          {enableUser.isError ? <div className="mt-2"><ErrorBox error={enableUser.error} /></div> : null}
        </Card>
      </div>
      {billing.isError ? <div className="mb-4"><ErrorBox error={billing.error} /></div> : null}
      {billing.data ? (
        <Card className="mb-4 mt-4">
          <h2 className="mb-3 font-medium">Кошелёк</h2>
          <p className="mb-3 text-sm">
            доступно {formatMoney(billing.data.available_balance, billing.data.currency)} · заморожено{" "}
            {formatMoney(billing.data.held_balance, billing.data.currency)}
          </p>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <h3 className="mb-2 text-sm font-medium">Пополнить</h3>
              <Field label="Сумма">
                <Input value={topupAmount} onChange={(e) => setTopupAmount(e.target.value)} />
              </Field>
              <Field label="Комментарий">
                <Input value={topupComment} onChange={(e) => setTopupComment(e.target.value)} />
              </Field>
              {topup.isError ? <ErrorBox error={topup.error} /> : null}
              <Button type="button" disabled={topup.isPending || !topupAmount} onClick={() => topup.mutate()}>
                Пополнить
              </Button>
            </div>
            <div>
              <h3 className="mb-2 text-sm font-medium">Корректировка</h3>
              <Field label="Направление">
                <Select value={adjustDir} onChange={(e) => setAdjustDir(e.target.value)}>
                  <option value="credit">плюс</option>
                  <option value="debit">минус</option>
                </Select>
              </Field>
              <Field label="Сумма">
                <Input value={adjustAmount} onChange={(e) => setAdjustAmount(e.target.value)} />
              </Field>
              <Field label="Комментарий (обязательно)">
                <Input value={adjustComment} onChange={(e) => setAdjustComment(e.target.value)} />
              </Field>
              <label className="mb-3 flex items-center gap-2 text-sm">
                <input type="checkbox" checked={allowNeg} onChange={(e) => setAllowNeg(e.target.checked)} />
                разрешить минус
              </label>
              {adjust.isError ? <ErrorBox error={adjust.error} /> : null}
              <Button
                variant="secondary"
                type="button"
                disabled={adjust.isPending || !adjustAmount || !adjustComment}
                onClick={() => adjust.mutate()}
              >
                Применить
              </Button>
            </div>
          </div>
          <h3 className="mt-4 mb-2 text-sm font-medium">Тарифы</h3>
          <p className="mb-3 text-xs text-zinc-500">
            SMS / Russia — номера 7… как в направлениях платформы (включая 77…), не географическая Россия. HLR Lookup и
            Silent SMS назначаются отдельно, цена — за проверку, без себестоимости.
          </p>
          <div className="mb-3 grid gap-3 md:grid-cols-2">
            {TARIFF_PRODUCTS.map((product) => {
              const current = assigned[product];
              const label = productLabel[product] ?? product;
              const planID = assignPlans[product] ?? "";
              const plans = (tariffs.data?.items ?? []).filter((p) => p.product === product && p.is_active);
              return (
                <div key={product} className="rounded-lg border border-zinc-200 p-3">
                  <div className="font-medium">Тариф {label}</div>
                  <div className="mt-1 text-sm text-zinc-500">
                    {current
                      ? `${current.plan_name} — ${formatMoney(current.sell_price, current.currency)} ${priceUnit(product)}${current.is_active === false ? " · план неактивен" : ""}`
                      : "Не назначен"}
                  </div>
                  <div className="mt-3 flex flex-wrap gap-2">
                    <Select
                      value={planID}
                      onChange={(e) => setAssignPlans((cur) => ({ ...cur, [product]: e.target.value }))}
                    >
                      <option value="">{plans.length === 0 ? "нет активных планов" : "Выберите..."}</option>
                      {plans.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name} · {formatMoney(p.sell_price, p.currency)}
                        </option>
                      ))}
                    </Select>
                    <Button
                      type="button"
                      disabled={!planID || assign.isPending}
                      onClick={() => {
                        if (current && !window.confirm(`Заменить тариф «${label}»?`)) {
                          return;
                        }
                        assign.mutate({ product, tariff_plan_id: planID });
                      }}
                    >
                      Назначить
                    </Button>
                  </div>
                  <Button
                    className="mt-2"
                    variant="secondary"
                    type="button"
                    disabled={!current || unassign.isPending}
                    onClick={() => {
                      if (window.confirm(`Снять тариф «${label}» с клиента?`)) {
                        unassign.mutate(product);
                      }
                    }}
                  >
                    Снять
                  </Button>
                </div>
              );
            })}
          </div>
          <p className="mb-3 text-xs">
            <Link className="text-blue-700 hover:underline" to={`/jobs?client_id=${id}`}>
              Задания клиента
            </Link>
          </p>
          {assign.isError ? <ErrorBox error={assign.error} /> : null}
          {unassign.isError ? <ErrorBox error={unassign.error} /> : null}
          <Table>
            <thead>
              <tr>
                <Th>Время</Th>
                <Th>Тип</Th>
                <Th>Сумма</Th>
                <Th>Комментарий</Th>
              </tr>
            </thead>
            <tbody>
              {billing.data.ledger.map((row) => (
                <tr key={row.id}>
                  <Td>{formatDateTime(row.created_at)}</Td>
                  <Td>{row.type}</Td>
                  <Td>{formatMoney(row.amount, row.currency)}</Td>
                  <Td>{row.description ?? "—"}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Card>
      ) : null}
      <Card className="mt-4">
        <h2 className="mb-3 font-medium">API-ключи</h2>
        {createdToken ? (
          <Alert tone="amber">
            Секрет показывается один раз: <code className="break-all text-xs">{createdToken}</code>
          </Alert>
        ) : null}
        <div className="mt-3 grid gap-3 md:grid-cols-3">
          <Field label="Имя">
            <Input value={keyName} onChange={(e) => setKeyName(e.target.value)} />
          </Field>
          <Field label="CIDR (пусто = любой IP)">
            <Input value={cidrs} onChange={(e) => setCidrs(e.target.value)} placeholder="203.0.113.0/24" />
          </Field>
          <div className="md:col-span-3 mb-3 flex flex-wrap gap-3 text-sm">
            {allScopes.map((s) => (
              <label key={s} className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={scopes.includes(s)}
                  onChange={(e) =>
                    setScopes((cur) => (e.target.checked ? [...cur, s] : cur.filter((x) => x !== s)))
                  }
                />
                {s}
              </label>
            ))}
          </div>
          <div className="flex items-end">
            <Button type="button" onClick={() => createKey.mutate()} disabled={createKey.isPending || scopes.length === 0}>
              Выпустить ключ
            </Button>
          </div>
        </div>
        {createKey.isError ? <ErrorBox error={createKey.error} /> : null}
        <Table>
          <thead>
            <tr>
              <Th>Префикс</Th>
              <Th>Имя</Th>
              <Th>Статус</Th>
              <Th>Последнее использование</Th>
              <Th></Th>
            </tr>
          </thead>
          <tbody>
            {(keys.data?.items ?? []).map((k) => (
              <tr key={k.id}>
                <Td>
                  <code>{k.key_prefix}</code>
                </Td>
                <Td>{k.name}</Td>
                <Td>
                  <Badge tone={statusTone(k.status)}>{k.status}</Badge>
                </Td>
                <Td>{formatDateTime(k.last_used_at)}</Td>
                <Td>
                  {k.status === "active" ? (
                    <Button variant="ghost" type="button" onClick={() => revoke.mutate(k.id)}>
                      Отозвать
                    </Button>
                  ) : null}
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
        {!keys.isLoading && (keys.data?.items ?? []).length === 0 ? <EmptyState>Ключей нет</EmptyState> : null}
      </Card>
    </div>
  );
}
