export const campaignStatusLabel: Record<string, string> = {
  draft: "черновик",
  queued: "в очереди",
  running: "идёт рассылка",
  completed: "завершена",
  failed: "ошибка",
  cancelled: "отменена",
};

export const smsStatusLabel: Record<string, string> = {
  queued: "в очереди",
  accepted: "принято",
  sent: "отправлено",
  delivered: "доставлено",
  failed: "ошибка",
};

export const recipientStatusLabel: Record<string, string> = {
  pending: "ожидает",
  enqueued: "поставлено",
  skipped: "пропущено",
  failed: "ошибка",
};

export const sendJobStatusLabel: Record<string, string> = {
  pending: "ожидает",
  processing: "отправка",
  done: "готово",
  retry: "повтор",
  uncertain: "неясно",
  dead: "остановлено",
};

export const billingStateLabel: Record<string, string> = {
  none: "не тарифицируется",
  hold: "холд открыт",
  capture: "списано",
  release: "холд снят",
};

export function campaignInflight(status?: string) {
  return status === "queued" || status === "running";
}

export function smsInflight(status?: string) {
  return status === "queued" || status === "accepted" || status === "sent";
}
