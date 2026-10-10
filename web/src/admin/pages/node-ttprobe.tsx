import { useMutation, useQuery } from "@tanstack/react-query";
import { Download, ShieldCheck } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api, errorText, unwrap, type Schemas } from "../../api/client";
import { Drawer } from "../../components/overlay";
import { Button, Field, Pill } from "../../components/ui";
import { t } from "../../i18n";
import { nodeLabel } from "../../lib/node-label";

type Result = Schemas["TTProbeView"];
type Node = Schemas["NodeInfo"];

export function TTProbeDrawer({ node, onClose }: { node: Node | null; onClose: () => void }) {
  return <Drawer open={!!node} onOpenChange={(v) => !v && onClose()} title={t("ttProbe.title")} meta={node ? nodeLabel(node) : undefined} wide>
    {node ? <ProbeForm key={node.id} node={node} /> : null}
  </Drawer>;
}

function ProbeForm({ node }: { node: Node }) {
  const all = useQuery({ queryKey: ["inbounds"], queryFn: ({ signal }) => unwrap(api.GET("/api/v1/inbounds", { signal })) });
  const choices = (all.data ?? []).filter((i) => i.node_id === node.id && i.type === "trusttunnel" && i.enabled);
  const [picked, setPicked] = useState(0);
  const [reference, setReference] = useState("8444");
  const [result, setResult] = useState<Result | null>(null);
  const [formError, setFormError] = useState("");
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const inbound = choices.find((i) => i.id === picked) ?? choices[0];
  const run = useMutation({
    mutationFn: () => {
      controller.current = new AbortController();
      return unwrap(api.POST("/api/v1/nodes/{id}/trusttunnel-probe", {
        params: { path: { id: node.id } },
        body: { inbound_id: inbound!.id, reference_port: reference.trim() === "" ? 0 : Number(reference) },
        signal: controller.current.signal,
      }));
    },
    onSuccess: (r) => setResult(r),
    onError: (e) => setFormError(errorText(e)),
  });
  const start = () => {
    setFormError("");
    if (reference.trim() !== "" && (!/^\d+$/.test(reference) || Number(reference) < 1 || Number(reference) > 65535)) { setFormError(t("ttProbe.badPort")); return; }
    setResult(null); run.mutate();
  };
  const download = () => {
    if (!result) return;
    const url = URL.createObjectURL(new Blob([JSON.stringify(result, null, 2)], { type: "application/json" }));
    const a = document.createElement("a"); a.href = url; a.download = `kinkan-probe-node-${node.id}.json`; a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return <div className="space-y-5 pt-5">
    <p className="text-[13px] text-[var(--ink-500)]">{t("ttProbe.intro")}</p>
    <div className="banner info">{t("ttProbe.vantage")}</div>
    {all.isError ? <p role="alert">{errorText(all.error)}</p> : all.isPending ? <p role="status">{t("common.loading")}</p> : choices.length === 0 ? <p>{t("ttProbe.noInbound")}</p> : <>
      <Field label={t("ttProbe.inbound")} htmlFor="tt-probe-inbound"><select id="tt-probe-inbound" className="input" value={inbound?.id ?? ""} disabled={run.isPending} onChange={(e) => setPicked(Number(e.target.value))}>
        {choices.map((i) => <option key={i.id} value={i.id}>{i.name} · {i.port}</option>)}
      </select></Field>
      <Field label={t("ttProbe.reference")} htmlFor="tt-probe-reference" hint={t("ttProbe.referenceHint")}><input id="tt-probe-reference" className="input" type="text" inputMode="numeric" placeholder="8444" value={reference} disabled={run.isPending} onChange={(e) => setReference(e.target.value)} /></Field>
      <Button variant="primary" loading={run.isPending} disabled={!node.enabled || !inbound} onClick={start}><ShieldCheck size={16} aria-hidden />{run.isPending ? t("ttProbe.running") : t("ttProbe.run")}</Button>
    </>}
    {run.isPending ? <p role="status" aria-live="polite" className="text-sm">{t("ttProbe.wait")}</p> : null}
    {formError ? <p role="alert" className="text-sm text-[var(--berry-600)]">{formError}</p> : null}
    {result ? <>
      <div className="flex flex-wrap gap-2" aria-live="polite">{(["FAIL", "ERROR", "WARN", "PASS", "INFO"] as const).map((level) => <Pill key={level} tone={level === "PASS" ? "ok" : level === "WARN" ? "warn" : level === "INFO" ? "off" : "bad"}>{t(`ttProbe.level${level}`)}: {result.report.findings.filter((f) => f.level === level).length}</Pill>)}</div>
      <dl className="break-all text-xs"><dt>{t("ttProbe.target")}</dt><dd className="mono mb-2">{result.report.target}</dd><dt>{t("ttProbe.cover")}</dt><dd className="mono">{result.report.reference || t("ttProbe.noReference")}</dd></dl>
      <Button onClick={download}><Download size={16} aria-hidden />{t("ttProbe.download")}</Button>
      <p className="text-xs text-[var(--ink-500)]">{t("ttProbe.limits")}</p>
      <div className="space-y-2">{result.report.findings.map((f, i) => <details key={i} className="rounded-xl border border-[var(--hairline)] p-3" open={f.level === "FAIL" || f.level === "ERROR"}>
        <summary className="cursor-pointer break-words text-sm"><span className={f.level === "FAIL" || f.level === "ERROR" ? "text-[var(--berry-600)]" : ""}>{f.level}</span> · {f.name}</summary>
        <p className="mono mt-2 whitespace-pre-wrap break-all text-xs text-[var(--ink-500)]">{f.detail}</p>
      </details>)}</div>
    </> : null}
  </div>;
}
