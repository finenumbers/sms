import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Alert, Badge, Button, Card, ErrorBox, Field, Input, PageHeader, formatMoney } from "ui";
import { api, probeSMSCConnectivity, type SMSCBalance, type SMSCConnectivity, type Settings } from "../api";
import { yn } from "../lookup";

type Form = Partial<Settings> & {
  runexis_password?: string;
  rotate_ingress_token?: boolean;
  smsc_apikey?: string;
  smsc_callback_secret?: string;
};

export function SettingsPage() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["settings"], queryFn: () => api.get<Settings>("/settings") });
  const [form, setForm] = useState<Form>({});
  const [token, setToken] = useState("");
  useEffect(() => {
    if (q.data) {
      setForm(q.data);
    }
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      api.patch<Settings>("/settings", {
        runexis_email: form.runexis_email,
        runexis_password: form.runexis_password || undefined,
        callback_base_url: form.callback_base_url,
        sms_directions: form.sms_directions,
        provider_rps: form.provider_rps,
        client_rps_default: form.client_rps_default,
        retention_days: form.retention_days,
        audit_retention_days: form.audit_retention_days,
        ops_retention_days: form.ops_retention_days,
        low_balance_threshold: form.low_balance_threshold,
        rotate_ingress_token: form.rotate_ingress_token || undefined,
        lookup_enabled: form.lookup_enabled,
        lookup_check_timeout_sec: form.lookup_check_timeout_sec,
        lookup_poll_interval_sec: form.lookup_poll_interval_sec,
        lookup_max_csv_rows: form.lookup_max_csv_rows,
        lookup_max_csv_bytes: form.lookup_max_csv_bytes,
        lookup_max_batch_phones: form.lookup_max_batch_phones,
        lookup_webhook_max_attempts: form.lookup_webhook_max_attempts,
        lookup_webhook_timeout_ms: form.lookup_webhook_timeout_ms,
        lookup_retention_days: form.lookup_retention_days,
        smsc_base_url: form.smsc_base_url,
        smsc_apikey: form.smsc_apikey || undefined,
        smsc_callback_secret: form.smsc_callback_secret || undefined,
        smsc_currency: form.smsc_currency,
      }),
    onSuccess: (view) => {
      void qc.setQueryData(["settings"], view);
      setForm({ ...view, runexis_password: "", smsc_apikey: "", smsc_callback_secret: "" });
    },
  });
  const test = useMutation({
    mutationFn: () =>
      api.post<{ ok: boolean; email: string; name: string; statistic_ok: boolean; statistic_error?: string }>(
        "/settings/runexis/test",
      ),
  });
  const register = useMutation({
    mutationFn: () => api.post<{ ok: boolean; dlr_url: string; hook_url: string }>("/settings/runexis/callbacks", { ingress_token: token }),
  });
  const smscProbe = useMutation({ mutationFn: () => probeSMSCConnectivity() });
  const smscBalance = useMutation({ mutationFn: () => api.get<SMSCBalance>("/provider/smsc/balance") });

  if (q.isError) {
    return <ErrorBox error={q.error} />;
  }
  if (!q.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const d = form.sms_directions ?? q.data.sms_directions;
  const probe: SMSCConnectivity | undefined = smscProbe.data;
  return (
    <div>
      <PageHeader
        title="Настройки"
        actions={
          <Button type="button" onClick={() => save.mutate()} disabled={save.isPending}>
            Сохранить
          </Button>
        }
      />
      <p className="mb-3 text-sm text-zinc-500">
        Учётные данные SMS (Runexis) и HLR / Silent SMS (SMSC) задаются здесь, не в env. После сохранения проверка обмена
        не требует перезапуска.
      </p>
      {save.isError ? (
        <div className="mb-3">
          <ErrorBox error={save.error} />
        </div>
      ) : null}
      {save.isSuccess ? (
        <Alert className="mb-3" tone="green">
          Сохранено
        </Alert>
      ) : null}
      {save.data?.ingress_token ? (
        <Alert className="mb-3" tone="amber">
          Новый токен входящих (один раз): {save.data.ingress_token}
        </Alert>
      ) : null}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <h2 className="mb-3 font-medium">Runexis (SMS)</h2>
          <Field label="Эл. почта агента">
            <Input value={form.runexis_email ?? ""} onChange={(e) => setForm({ ...form, runexis_email: e.target.value })} />
          </Field>
          <Field label={q.data.runexis_password_set ? "Пароль (оставьте пустым, чтобы не менять)" : "Пароль"}>
            <Input type="password" value={form.runexis_password ?? ""} onChange={(e) => setForm({ ...form, runexis_password: e.target.value })} />
          </Field>
          <Field label="Базовый URL колбэков">
            <Input
              value={form.callback_base_url ?? ""}
              onChange={(e) => setForm({ ...form, callback_base_url: e.target.value })}
              placeholder="https://api.{domain}"
            />
          </Field>
          <p className="mb-3 text-xs text-zinc-500">
            Для DLR нужен публичный <code>{"https://api.{domain}"}</code> — тот же хост, что <code>API_HOST</code> в
            Portainer. <code>localhost</code> Runexis не достучится: исходящие тогда остаются accepted, пока statistic (~2
            мин) не подтвердит доставку.
          </p>
          <div className="mb-3 grid grid-cols-2 gap-2 text-sm">
            {(
              [
                ["in", "Входящие (in)"],
                ["dom_out", "Исходящие РФ (dom_out)"],
                ["int_out", "Международные (int_out)"],
                ["in_mass", "Входящие массовые (in_mass)"],
              ] as const
            ).map(([key, label]) => (
              <label key={key} className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={Boolean(d[key])}
                  onChange={(e) => setForm({ ...form, sms_directions: { ...d, [key]: e.target.checked } })}
                />
                {label}
              </label>
            ))}
          </div>
          <Field label="Лимит провайдера, SMS/с">
            <Input type="number" step="0.1" value={form.provider_rps ?? ""} onChange={(e) => setForm({ ...form, provider_rps: Number(e.target.value) })} />
          </Field>
          <Field label="Лимит клиента по умолчанию, SMS/с">
            <Input type="number" step="0.1" value={form.client_rps_default ?? ""} onChange={(e) => setForm({ ...form, client_rps_default: Number(e.target.value) })} />
          </Field>
          <Field label="Хранение SMS, дни">
            <Input type="number" value={form.retention_days ?? ""} onChange={(e) => setForm({ ...form, retention_days: Number(e.target.value) })} />
          </Field>
          <Field label="Хранение аудита, дни">
            <Input type="number" value={form.audit_retention_days ?? ""} onChange={(e) => setForm({ ...form, audit_retention_days: Number(e.target.value) })} />
          </Field>
          <Field label="Хранение журнала, дни (1–90)">
            <Input type="number" value={form.ops_retention_days ?? ""} onChange={(e) => setForm({ ...form, ops_retention_days: Number(e.target.value) })} />
          </Field>
          <Field label="Порог низкого баланса">
            <Input value={form.low_balance_threshold ?? ""} onChange={(e) => setForm({ ...form, low_balance_threshold: e.target.value })} />
          </Field>
          <label className="mb-3 flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={Boolean(form.rotate_ingress_token)}
              onChange={(e) => setForm({ ...form, rotate_ingress_token: e.target.checked })}
            />
            Сменить токен входящих колбэков
          </label>
        </Card>
        <Card>
          <h2 className="mb-3 font-medium">Проверка SMS (Runexis)</h2>
          <Button variant="secondary" type="button" onClick={() => test.mutate()} disabled={test.isPending}>
            Проверить обмен с DIDAPI
          </Button>
          {test.isSuccess ? (
            <div className="mt-2">
              <Alert tone="green">
                OK · {test.data.email}
                {test.data.name ? ` · ${test.data.name}` : ""}
                {test.data.statistic_ok ? " · statistic в порядке" : ` · ошибка statistic: ${test.data.statistic_error ?? "ошибка"}`}
              </Alert>
            </div>
          ) : null}
          {test.isError ? (
            <div className="mt-2">
              <ErrorBox error={test.error} />
            </div>
          ) : null}
          <div className="mt-4">
            <Field label="Токен входящих (для регистрации URL у провайдера)">
              <Input value={token} onChange={(e) => setToken(e.target.value)} />
            </Field>
            <Button type="button" onClick={() => register.mutate()} disabled={!token || register.isPending}>
              Зарегистрировать URL доставки и входящих
            </Button>
            {register.isSuccess ? (
              <div className="mt-2 text-xs text-zinc-600">
                {register.data.dlr_url}
                <br />
                {register.data.hook_url}
              </div>
            ) : null}
            {register.isError ? (
              <div className="mt-2">
                <ErrorBox error={register.error} />
              </div>
            ) : null}
          </div>
          <p className="mt-4 text-xs text-zinc-500">
            Токен входящих задан: {q.data.ingress_token_set ? "да" : "нет"}. DEK: {q.data.dek_key_id ?? "—"}.
          </p>
        </Card>
        <Card>
          <h2 className="mb-3 font-medium">SMSC (HLR и Silent SMS)</h2>
          <p className="mb-3 text-xs text-zinc-500">
            Тот же аккаунт, что у старого HLR. Авторизация — только API-ключ. Секрет колбэка обязан совпасть, иначе после
            смены URL все колбэки получат 401. Ключ и секрет шифруются ключом платформы.
          </p>
          <Field label="Базовый URL SMSC">
            <Input
              value={form.smsc_base_url ?? ""}
              onChange={(e) => setForm({ ...form, smsc_base_url: e.target.value })}
              placeholder="https://smsc.ru"
            />
          </Field>
          <Field label={q.data.smsc_apikey_set ? "API-ключ (оставьте пустым, чтобы не менять)" : "API-ключ"}>
            <Input type="password" value={form.smsc_apikey ?? ""} onChange={(e) => setForm({ ...form, smsc_apikey: e.target.value })} />
          </Field>
          <Field label={q.data.smsc_callback_secret_set ? "Секрет колбэка (оставьте пустым, чтобы не менять)" : "Секрет колбэка"}>
            <Input type="password" value={form.smsc_callback_secret ?? ""} onChange={(e) => setForm({ ...form, smsc_callback_secret: e.target.value })} />
          </Field>
          <Field label="Валюта кабинета SMSC">
            <Input value={form.smsc_currency ?? "RUB"} onChange={(e) => setForm({ ...form, smsc_currency: e.target.value })} />
          </Field>
          <p className="mb-3 text-xs text-zinc-600">
            URL колбэка (прописать вручную в кабинете SMSC.ru):{" "}
            <code className="break-all">{q.data.smsc_callback_url || "сначала задайте базовый URL колбэков"}</code>
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" type="button" disabled={smscProbe.isPending} onClick={() => smscProbe.mutate()}>
              Проверить связь
            </Button>
            <Button variant="secondary" type="button" disabled={smscBalance.isPending} onClick={() => smscBalance.mutate()}>
              Баланс SMSC
            </Button>
          </div>
          {probe ? (
            <ul className="mt-3 space-y-1 text-sm">
              <li>
                учётные данные: <Badge tone={probe.configured ? "green" : "red"}>{yn(probe.configured)}</Badge>
              </li>
              <li>
                секрет колбэка:{" "}
                <Badge tone={probe.callback_secret_configured ? "green" : "red"}>{yn(probe.callback_secret_configured)}</Badge>
              </li>
              <li>
                баланс: <Badge tone={probe.balance_ok ? "green" : "red"}>{yn(probe.balance_ok)}</Badge>
                {probe.balance ? ` · ${formatMoney(probe.balance, probe.currency)}` : null}
              </li>
              <li>
                подпись: <Badge tone={probe.signature_ok ? "green" : "red"}>{yn(probe.signature_ok)}</Badge>
              </li>
            </ul>
          ) : null}
          {probe?.balance_error ? <Alert className="mt-3">{probe.balance_error}</Alert> : null}
          {smscProbe.isError ? (
            <div className="mt-3">
              <ErrorBox error={smscProbe.error} />
            </div>
          ) : null}
          {smscBalance.data ? (
            <Alert className="mt-3" tone="green">
              Баланс SMSC: {formatMoney(smscBalance.data.balance, smscBalance.data.currency)}
            </Alert>
          ) : null}
          {smscBalance.isError ? (
            <div className="mt-3">
              <ErrorBox error={smscBalance.error} />
            </div>
          ) : null}
          <p className="mt-3 text-xs text-zinc-500">Проверка не отправляет HLR и Silent SMS. Submit только у воркера.</p>
        </Card>
        <Card>
          <h2 className="mb-3 font-medium">HLR и Silent SMS</h2>
          <p className="mb-3 text-xs text-zinc-500">
            Включать только после проверки HOLD→DEBIT/RELEASE и колбэка на стенде. Выключенный флаг не шлёт в SMSC;
            уже ушедшие pending дожимаются.
          </p>
          <label className="mb-3 flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={Boolean(form.lookup_enabled)}
              onChange={(e) => setForm({ ...form, lookup_enabled: e.target.checked })}
            />
            услуга включена
          </label>
          <Field label="Таймаут проверки, сек">
            <Input
              type="number"
              value={form.lookup_check_timeout_sec ?? ""}
              onChange={(e) => setForm({ ...form, lookup_check_timeout_sec: Number(e.target.value) })}
            />
          </Field>
          <Field label="Интервал опроса, сек">
            <Input
              type="number"
              value={form.lookup_poll_interval_sec ?? ""}
              onChange={(e) => setForm({ ...form, lookup_poll_interval_sec: Number(e.target.value) })}
            />
          </Field>
          <Field label="Потолок строк CSV">
            <Input
              type="number"
              value={form.lookup_max_csv_rows ?? ""}
              onChange={(e) => setForm({ ...form, lookup_max_csv_rows: Number(e.target.value) })}
            />
          </Field>
          <Field label="Потолок CSV, байт">
            <Input
              type="number"
              value={form.lookup_max_csv_bytes ?? ""}
              onChange={(e) => setForm({ ...form, lookup_max_csv_bytes: Number(e.target.value) })}
            />
          </Field>
          <Field label="Потолок номеров в списке">
            <Input
              type="number"
              value={form.lookup_max_batch_phones ?? ""}
              onChange={(e) => setForm({ ...form, lookup_max_batch_phones: Number(e.target.value) })}
            />
          </Field>
          <Field label="Попытки webhook">
            <Input
              type="number"
              value={form.lookup_webhook_max_attempts ?? ""}
              onChange={(e) => setForm({ ...form, lookup_webhook_max_attempts: Number(e.target.value) })}
            />
          </Field>
          <Field label="Таймаут webhook, мс">
            <Input
              type="number"
              value={form.lookup_webhook_timeout_ms ?? ""}
              onChange={(e) => setForm({ ...form, lookup_webhook_timeout_ms: Number(e.target.value) })}
            />
          </Field>
          <Field label="Хранение raw SMSC, дни">
            <Input
              type="number"
              value={form.lookup_retention_days ?? ""}
              onChange={(e) => setForm({ ...form, lookup_retention_days: Number(e.target.value) })}
            />
          </Field>
        </Card>
      </div>
    </div>
  );
}
