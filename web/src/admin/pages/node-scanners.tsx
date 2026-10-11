import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useState } from "react";
import { api, errorText, unwrap, type Schemas } from "../../api/client";
import { Confirm, Drawer } from "../../components/overlay";
import { useToast } from "../../components/toast";
import { Button, Pill } from "../../components/ui";
import { t } from "../../i18n";
import { nodeLabel } from "../../lib/node-label";

type Node = Schemas["NodeInfo"];
export function ScannerDrawer({ node, onClose }: { node: Node | null; onClose: () => void }) {
  return <Drawer open={!!node} onOpenChange={(v) => !v && onClose()} title={t("scanners.title")} meta={node ? nodeLabel(node) : undefined} wide>
    {node ? <Journal key={node.id} node={node} /> : null}
  </Drawer>;
}
function Journal({ node }: { node: Node }) {
  const [showClients, setShowClients] = useState(false);
  const [showOwn, setShowOwn] = useState(false);
  const result = useQuery({ queryKey: ["scanners", node.id], queryFn: ({ signal }) => unwrap(api.GET("/api/v1/nodes/{id}/scanners", { params: { path: { id: node.id } }, signal })), refetchInterval: 30_000 });
  if (result.isPending) return <p role="status">{t("common.loading")}</p>;
  if (result.isError) return <p role="alert">{errorText(result.error)}</p>;
  const grouped = new Map<string, Schemas["ScannerRecord"] & { methods: Set<string>; reasons: Set<string> }>();
  for (const row of result.data.records) {
    if (row.own ? !showOwn : row.client && !showClients) continue;
    const key = `${row.ip}/${row.inbound}/${row.protocol}/${row.client}/${!!row.own}`;
    const old = grouped.get(key);
    if (old) { old.count += row.count; old.first = Math.min(old.first, row.first); old.last = Math.max(old.last, row.last); old.methods.add(row.method ?? ""); old.reasons.add(row.reason); }
    else grouped.set(key, { ...row, methods: new Set([row.method ?? ""]), reasons: new Set([row.reason]) });
  }
  const rows = [...grouped.values()].sort((a, b) => b.count - a.count);
  const max = Math.max(1, ...result.data.days.map((d) => d.count));
  return <div className="space-y-5 pt-5">
    <p className="text-sm text-[var(--ink-500)]">{t("scanners.intro")}</p>
    <div className="banner info">{t("scanners.coverage")}</div>
    <CollectionStatus status={result.data.status} />
    <ClearJournal nodeId={node.id} />
    {result.data.spike ? <div role="alert" className="banner warn">{t("scanners.spike")}</div> : null}
    <figure><figcaption className="text-sm">{t("scanners.days")}</figcaption>
      <svg viewBox="0 0 620 130" role="img" aria-label={t("scanners.days")} className="w-full">
        {result.data.days.map((d, i) => <g key={d.day}><title>{new Date(d.day * 1000).toLocaleDateString()} · {d.count}</title><rect x={i * 20 + 10} y={110 - d.count / max * 100} width={14} height={Math.max(1, d.count / max * 100)} fill="currentColor" className="text-[var(--amber-500)]" /></g>)}
        <text x="10" y="128" fontSize="10" fill="currentColor">{new Date(result.data.days[0]!.day * 1000).toLocaleDateString()}</text><text x="600" y="128" fontSize="10" textAnchor="end" fill="currentColor">{new Date(result.data.days.at(-1)!.day * 1000).toLocaleDateString()}</text>
      </svg>
    </figure>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={showClients} onChange={(e) => setShowClients(e.target.checked)} />{t("scanners.showClients")}</label>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={showOwn} onChange={(e) => setShowOwn(e.target.checked)} />{t("scanners.showOwn")}</label>
    {rows.length === 0 ? <p>{t("scanners.empty")}</p> : <div className="overflow-x-auto"><table className="w-full text-left text-xs"><thead><tr><th>{t("scanners.source")}</th><th>{t("scanners.who")}</th><th>{t("scanners.inbound")}</th><th>{t("scanners.attempts")}</th><th>{t("scanners.when")}</th></tr></thead><tbody>
      {rows.map((r) => <tr key={`${r.ip}/${r.inbound}/${r.protocol}/${r.client}/${!!r.own}`} className="border-t border-[var(--hairline)] align-top">
        <td className="py-3 pr-3"><p className="mono">{r.ip}</p><p>{r.country || "—"} {r.asn ? `AS${r.asn}` : ""} {r.organization}</p><p className="break-all">{r.ptr}</p></td>
        <td className="py-3 pr-3"><Pill tone={r.own || r.client ? "off" : "warn"}>{r.own ? t("scanners.own") : r.client ? t("scanners.client") : r.scanner === "unknown" ? t("scanners.unknown") : r.scanner}</Pill><p>{r.evidence}</p></td>
        <td className="py-3 pr-3">{r.inbound} · {r.protocol}<p>{[...r.methods].filter(Boolean).join(", ")}</p><p>{[...r.reasons].join(", ")}</p></td>
        <td className="py-3 pr-3">{r.count}</td><td className="py-3">{new Date(r.first * 1000).toLocaleString()}<br />{new Date(r.last * 1000).toLocaleString()}</td>
      </tr>)}
    </tbody></table></div>}
    <p className="text-xs text-[var(--ink-500)]">{t("scanners.geoSource")} <a href="https://db-ip.com" target="_blank" rel="noreferrer" className="underline">IP Geolocation by DB-IP</a></p>
  </div>;
}

// How collecting the node's journal goes: an empty table is quiet only when the last pull
// worked and the node dropped nothing.
function CollectionStatus({ status }: { status: Schemas["ScannerStatusView"] }) {
  const when = (at?: string | null) => (at ? new Date(at).toLocaleString() : t("scanners.statusNever"));
  return <>
    {status.unsupported ? <div className="banner warn">{t("scanners.statusUnsupported")}</div> : null}
    {status.error ? <div role="alert" className="banner warn">{t("scanners.statusError", { error: status.error, last: when(status.last_ok) })}</div> : null}
    {status.dropped > 0 ? <div className="banner warn">{t("scanners.statusDropped", { n: status.dropped })}</div> : null}
    {!status.error && !status.unsupported ? <p className="text-xs text-[var(--ink-500)]">{t("scanners.statusUpdated", { at: when(status.last_ok) })}</p> : null}
  </>;
}

// Clearing goes to the node first (a new, empty journal), then the panel's copy: cleared on
// the panel only, the next collection would bring the rows back.
function ClearJournal({ nodeId }: { nodeId: number }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [asking, setAsking] = useState(false);
  const clear = useMutation({
    mutationFn: () => unwrap(api.DELETE("/api/v1/nodes/{id}/scanners", { params: { path: { id: nodeId } } })),
    onSuccess: async () => { setAsking(false); toast.ok(t("scanners.cleared")); await qc.invalidateQueries({ queryKey: ["scanners", nodeId] }); },
    onError: (e) => { setAsking(false); toast.error(errorText(e)); },
  });
  return <>
    <Button size="sm" onClick={() => setAsking(true)}><Trash2 size={16} aria-hidden />{t("scanners.clear")}</Button>
    <Confirm open={asking} onOpenChange={setAsking} title={t("scanners.clearTitle")} text={t("scanners.clearText")} confirm={t("scanners.clear")} danger loading={clear.isPending} onConfirm={() => clear.mutate()} />
  </>;
}
