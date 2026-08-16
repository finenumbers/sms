import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { pollStatus } from "ui";
import { api, type Message } from "../api";
import { MessageDetailSheet } from "./MessageDetailSheet";

export function MessageDetailPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const q = useQuery({
    queryKey: ["message", id],
    queryFn: () => api.get<Message>(`/messages/${id}`),
    refetchInterval: pollStatus<Message>(),
    enabled: Boolean(id),
  });
  const m = q.data ?? null;

  return (
    <MessageDetailSheet
      open
      onOpenChange={(next) => {
        if (!next) {
          navigate(m?.direction === "inbound" ? "/inbox" : "/messages", { replace: true });
        }
      }}
      message={m}
      loading={q.isLoading}
      error={q.isError ? q.error : undefined}
    />
  );
}
