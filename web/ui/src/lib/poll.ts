const inflight = new Set(["queued", "accepted", "sent", "running"]);

export function pollStatus<T extends { status?: string }>(ms = 2000) {
  return (q: { state: { data?: T } }): number | false => {
    const status = q.state.data?.status;
    return status && inflight.has(status) ? ms : false;
  };
}
