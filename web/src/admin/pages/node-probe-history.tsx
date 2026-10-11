import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, Trash2 } from "lucide-react";
import { useState } from "react";
import { api, errorText, unwrap, type Schemas } from "../../api/client";
import { Confirm } from "../../components/overlay";
import { useToast } from "../../components/toast";
import { Button, Pill } from "../../components/ui";
import { t, tMaybe } from "../../i18n";
import { dateLong, time } from "../../lib/format";

// Kinkan: the node's earlier protocol checks, kept 30 days: open one again, delete it, or
// clear the node's whole history.

type Run = Schemas["ProbeRunView"];
type Result = Schemas["NodeProbeView"];

export const probeHistoryKey = (nodeId: number) => ["protocol-probes", nodeId] as const;

const tone = (v: string) => (v === "quiet" ? "ok" : v === "exposed" ? "bad" : "warn");

export function ProbeHistory({ nodeId, inboundName, onOpen }: { nodeId: number; inboundName: (id: number) => string | undefined; onOpen: (r: Result) => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [clearing, setClearing] = useState(false);
  const runs = useQuery({ queryKey: probeHistoryKey(nodeId), queryFn: ({ signal }) => unwrap(api.GET("/api/v1/nodes/{id}/protocol-probes", { params: { path: { id: nodeId } }, signal })) });
  const refresh = () => qc.invalidateQueries({ queryKey: probeHistoryKey(nodeId) });
  const open = useMutation({
    mutationFn: (id: number) => unwrap(api.GET("/api/v1/protocol-probes/{id}", { params: { path: { id } } })),
    onSuccess: onOpen,
    onError: (e) => toast.error(errorText(e)),
  });
  const remove = useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE("/api/v1/protocol-probes/{id}", { params: { path: { id } } })),
    onSuccess: refresh,
    onError: (e) => toast.error(errorText(e)),
  });
  const clear = useMutation({
    mutationFn: () => unwrap(api.DELETE("/api/v1/nodes/{id}/protocol-probes", { params: { path: { id: nodeId } } })),
    onSuccess: async () => { setClearing(false); toast.ok(t("probeHistory.cleared")); await refresh(); },
    onError: (e) => toast.error(errorText(e)),
  });
  const list = runs.data ?? [];
  return <section className="space-y-2" aria-labelledby="probe-history-title">
    <div className="flex flex-wrap items-center gap-2">
      <h3 id="probe-history-title" className="text-sm font-medium">{t("probeHistory.title")}</h3>
      {list.length > 0 ? <Button size="sm" className="ml-auto" onClick={() => setClearing(true)}><Trash2 size={16} aria-hidden />{t("probeHistory.clear")}</Button> : null}
    </div>
    <p className="text-xs text-[var(--ink-500)]">{t("probeHistory.intro")}</p>
    {runs.isError ? <p role="alert">{errorText(runs.error)}</p> : runs.isPending ? <p role="status">{t("common.loading")}</p> : list.length === 0 ? <p className="text-sm">{t("probeHistory.empty")}</p> :
      <ul className="space-y-1">{list.map((r: Run) => <li key={r.id} className="flex flex-wrap items-center gap-2 rounded-xl border border-[var(--hairline)] px-3 py-2 text-sm">
        <span>{dateLong(r.started_at)} {time(r.started_at)}</span>
        <Pill tone={tone(r.verdict)}>{tMaybe(`quietNode.level.${r.verdict}`) ?? tMaybe(`nodeProbe.${r.verdict}`) ?? r.verdict}</Pill>
        <span className="text-[var(--ink-500)]">{r.inbound_id ? (inboundName(r.inbound_id) ?? `#${r.inbound_id}`) : t("probeHistory.all", { n: r.inbounds })}</span>
        <span className="ml-auto flex gap-1">
          <Button size="sm" loading={open.isPending && open.variables === r.id} onClick={() => open.mutate(r.id)} aria-label={t("probeHistory.open")}><Eye size={16} aria-hidden /></Button>
          <Button size="sm" loading={remove.isPending && remove.variables === r.id} onClick={() => remove.mutate(r.id)} aria-label={t("probeHistory.delete")}><Trash2 size={16} aria-hidden /></Button>
        </span>
      </li>)}</ul>}
    <Confirm open={clearing} onOpenChange={setClearing} title={t("probeHistory.clearTitle")} text={t("probeHistory.clearText", { n: list.length })} confirm={t("probeHistory.clear")} danger loading={clear.isPending} onConfirm={() => clear.mutate()} />
  </section>;
}
