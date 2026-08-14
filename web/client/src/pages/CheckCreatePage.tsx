import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Alert, Button, Card, Field, Input, PageHeader, Textarea, formatMoney } from "ui";
import { api, type LookupCheckType, type LookupEstimate, type LookupJob, type LookupPreview } from "../api";
import { lookupError, parsePhoneList, typeLabel } from "../lookup";

export function CheckCreatePage({ type }: { type: LookupCheckType }) {
  const nav = useNavigate();
  const qc = useQueryClient();
  const [mode, setMode] = useState<"single" | "list" | "csv">("single");
  const [phone, setPhone] = useState("");
  const [list, setList] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<LookupPreview | null>(null);

  const phones = mode === "single" ? (phone.trim() ? [phone.trim()] : []) : parsePhoneList(list);
  const estimate = useQuery({
    queryKey: ["lookup-estimate", type, mode, phones],
    queryFn: () =>
      api.post<LookupEstimate>("/lookups/estimate", mode === "single" ? { type, phone: phones[0] } : { type, phones }),
    enabled: mode !== "csv" && phones.length > 0,
    retry: false,
  });
  const csvEstimate = useQuery({
    queryKey: ["lookup-csv-estimate", preview?.id],
    queryFn: () => api.post<LookupEstimate>(`/lookups/csv-previews/${preview!.id}/estimate`),
    enabled: Boolean(preview?.id),
    retry: false,
  });

  const upload = useMutation({
    mutationFn: (f: File) => api.upload<LookupPreview>("/lookups/csv-previews", f, "file", { type }),
    onSuccess: (row) => setPreview(row),
  });
  const submitSingle = useMutation({
    mutationFn: () => api.post<LookupJob>("/lookups/checks", { type, phone: phones[0] }),
    onSuccess: goJob,
  });
  const submitList = useMutation({
    mutationFn: () => api.post<LookupJob>("/lookups/jobs", { type, phones }),
    onSuccess: goJob,
  });
  const submitCSV = useMutation({
    mutationFn: () => api.post<LookupJob>(`/lookups/csv-previews/${preview!.id}/submit`),
    onSuccess: goJob,
  });

  function goJob(job: LookupJob) {
    void qc.invalidateQueries({ queryKey: ["lookup-jobs"] });
    void qc.invalidateQueries({ queryKey: ["balance"] });
    nav(`/lookups/${job.id}`);
  }

  const est = mode === "csv" ? csvEstimate : estimate;
  const pending = submitSingle.isPending || submitList.isPending || submitCSV.isPending || upload.isPending;
  const actionError = submitSingle.error ?? submitList.error ?? submitCSV.error ?? upload.error ?? est.error;

  return (
    <div>
      <PageHeader
        title={`Проверка ${typeLabel[type]}`}
        actions={
          <Link className="text-sm text-blue-700 hover:underline" to="/lookups">
            История проверок
          </Link>
        }
      />
      <Card className="mb-4">
        <div className="mb-3 flex flex-wrap gap-2 text-sm">
          {(
            [
              ["single", "Один номер"],
              ["list", "Список"],
              ["csv", "CSV"],
            ] as const
          ).map(([id, label]) => (
            <Button
              key={id}
              type="button"
              variant={mode === id ? "primary" : "secondary"}
              onClick={() => {
                setMode(id);
                setPreview(null);
              }}
            >
              {label}
            </Button>
          ))}
        </div>
        {mode === "single" ? (
          <Field label="Номер (+79…)">
            <Input value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+79001234567" />
          </Field>
        ) : null}
        {mode === "list" ? (
          <Field label="Номера, по одному в строке или через запятую. Только +79…">
            <Textarea value={list} onChange={(e) => setList(e.target.value)} rows={8} />
          </Field>
        ) : null}
        {mode === "csv" ? (
          <Field label="Файл CSV с номерами">
            <Input
              type="file"
              accept=".csv,text/csv,text/plain"
              onChange={(e) => {
                const next = e.target.files?.[0] ?? null;
                setFile(next);
                setPreview(null);
              }}
            />
          </Field>
        ) : null}
        {est.data ? (
          <p className="mb-3 text-sm text-zinc-600">
            {est.data.quantity} шт. × {formatMoney(est.data.unit_sell_price, est.data.currency)} ={" "}
            <span className="font-medium">{formatMoney(est.data.estimated_cost, est.data.currency)}</span>
            {preview ? ` · ${preview.phone_count} номеров в файле` : null}
          </p>
        ) : null}
        {actionError ? <Alert className="mb-3">{lookupError(actionError)}</Alert> : null}
        <div className="flex flex-wrap gap-2">
          {mode === "csv" && !preview ? (
            <Button
              type="button"
              disabled={!file || upload.isPending}
              onClick={() => file && upload.mutate(file)}
            >
              Загрузить и оценить
            </Button>
          ) : (
            <Button
              type="button"
              disabled={pending || (mode !== "csv" ? phones.length === 0 : !preview) || !est.data}
              onClick={() => {
                if (mode === "single") {
                  submitSingle.mutate();
                } else if (mode === "list") {
                  submitList.mutate();
                } else {
                  submitCSV.mutate();
                }
              }}
            >
              Запустить проверку
            </Button>
          )}
        </div>
      </Card>
      <p className="text-xs text-zinc-500">
        Пилот: только российские мобильные +79. Списание — после результата провайдера. Пока нет тарифа, кнопка покажет
        «услуга не подключена».
      </p>
    </div>
  );
}
