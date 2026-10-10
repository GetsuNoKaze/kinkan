import { useMutation, useQuery } from "@tanstack/react-query";
import { Download, ShieldCheck } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api, errorText, unwrap, type Schemas } from "../../api/client";
import { Drawer } from "../../components/overlay";
import { Button, Field, Pill } from "../../components/ui";
import { t } from "../../i18n";
import { dateLong, time } from "../../lib/format";
import { nodeLabel } from "../../lib/node-label";
import { QuietSummary } from "./node-quiet";

type Result = Schemas["NodeProbeView"];
type SingleResult = Schemas["TTProbeView"];
type Node = Schemas["NodeInfo"];
type Finding = Schemas["Finding"];
type ProtocolReport = Schemas["ProtocolProbeReport"];
type Fingerprint = Schemas["H2Fingerprint"];

// Worst first: a proxy sign or an unfinished check must not hide among 36 rows.
const LEVELS = ["FAIL", "ERROR", "WARN", "INFO", "PASS"] as const;
type Level = (typeof LEVELS)[number];
const rank = (level: string) => {
  const i = LEVELS.indexOf(level as Level);
  return i < 0 ? LEVELS.length : i;
};
const levelLabel = (level: string) => (LEVELS.includes(level as Level) ? t(`ttProbe.level${level as Level}`) : level);
const tone = (level: string) => (level === "PASS" ? "ok" : level === "WARN" ? "warn" : level === "INFO" ? "off" : "bad");

export function TTProbeDrawer({ node, onClose, onOpenSite }: { node: Node | null; onClose: () => void; onOpenSite?: () => void }) {
  return <Drawer open={!!node} onOpenChange={(v) => !v && onClose()} title={t("ttProbe.title")} meta={node ? nodeLabel(node) : undefined} wide>
    {node ? <ProbeForm key={node.id} node={node} onOpenSite={onOpenSite} /> : null}
  </Drawer>;
}

function ProbeForm({ node, onOpenSite }: { node: Node; onOpenSite?: () => void }) {
  const all = useQuery({ queryKey: ["inbounds"], queryFn: ({ signal }) => unwrap(api.GET("/api/v1/inbounds", { signal })) });
  const choices = (all.data ?? []).filter((i) => i.node_id === node.id && i.enabled);
  const [picked, setPicked] = useState(0);
  const [reference, setReference] = useState("");
  const [referenceHost, setReferenceHost] = useState("");
  const [result, setResult] = useState<Result | null>(null);
  const [formError, setFormError] = useState("");
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const inbound = choices.find((i) => i.id === picked);
  const run = useMutation({
    mutationFn: () => {
      controller.current = new AbortController();
      return unwrap(api.POST("/api/v1/nodes/{id}/protocol-probe", {
        params: { path: { id: node.id } },
        body: { inbound_id: picked, reference_host: referenceHost.trim() || undefined, reference_port: reference.trim() === "" ? 0 : Number(reference) },
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
      <Field label={t("ttProbe.inbound")} htmlFor="tt-probe-inbound"><select id="tt-probe-inbound" className="input" value={picked} disabled={run.isPending} onChange={(e) => setPicked(Number(e.target.value))}>
        <option value={0}>{t("nodeProbe.all")}</option>
        {choices.map((i) => <option key={i.id} value={i.id}>{i.name} · {i.type} · {i.port}</option>)}
      </select></Field>
      <Field label={t("ttProbe.reference")} htmlFor="tt-probe-reference" hint={t("ttProbe.referenceHint")}><input id="tt-probe-reference" className="input" type="text" inputMode="numeric" value={reference} disabled={run.isPending} onChange={(e) => setReference(e.target.value)} /></Field>
      <Field label={t("nodeProbe.referenceHost")} htmlFor="tt-probe-reference-host" hint={t("nodeProbe.referenceHostHint")}><input id="tt-probe-reference-host" className="input" type="text" value={referenceHost} disabled={run.isPending} maxLength={253} onChange={(e) => setReferenceHost(e.target.value)} /></Field>
      {reference.trim() === "" ? <div className="banner info">{t("nodeProbe.noReference")}</div> : null}
      <Button variant="primary" loading={run.isPending} disabled={!node.enabled || (picked !== 0 && !inbound)} onClick={start}><ShieldCheck size={16} aria-hidden />{run.isPending ? t("ttProbe.running") : t("ttProbe.run")}</Button>
    </>}
    {run.isPending ? <p role="status" aria-live="polite" className="text-sm">{t("ttProbe.wait")}</p> : null}
    {formError ? <p role="alert" className="text-sm text-[var(--berry-600)]">{formError}</p> : null}
    {result ? <>
      <QuietSummary quiet={result.quiet} onSite={onOpenSite} onReference={() => {
        // The node's own site on 443 is the usual reference (REALITY shows it there).
        setReference("443");
        document.getElementById("tt-probe-reference")?.focus();
      }} />
      <Button onClick={download}><Download size={16} aria-hidden />{t("ttProbe.download")}</Button>
      <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr><th>{t("ttProbe.inbound")}</th><th>{t("nodeProbe.verdict")}</th></tr></thead>
        <tbody>{result.inbounds.map((item) => <tr key={item.inbound_id}><td>{item.name} · {item.report.protocol} · {item.port}/{item.network}</td><td><Pill tone={item.report.verdict === "quiet" ? "ok" : item.report.verdict === "exposed" ? "bad" : "warn"}>{verdictLabel(item.report.verdict)}</Pill>{item.report.incomplete ? ` · ${t("nodeProbe.incomplete")}` : ""}<p className="mt-1 text-xs text-[var(--ink-500)]">{reportReason(item.report)}</p></td></tr>)}</tbody>
      </table></div>
      {result.inbounds.map((item) => <details key={item.inbound_id} className="rounded-xl border border-[var(--hairline)] p-3"><summary className="cursor-pointer text-sm">{item.name} · {verdictLabel(item.report.verdict)}</summary>
        <Report result={{ ...result, inbound_id: item.inbound_id, report: item.report }} onDownload={download} />
      </details>)}
    </> : null}
  </div>;
}

function Report({ result, onDownload }: { result: SingleResult; onDownload: () => void }) {
  const [filter, setFilter] = useState<Level | null>(null);
  const findings = result.report.findings;
  const count = (level: Level) => findings.filter((f) => f.level === level).length;
  const sorted = findings.map((f, i) => ({ f, i })).sort((a, b) => rank(a.f.level) - rank(b.f.level) || a.i - b.i);
  const shown = filter ? sorted.filter(({ f }) => f.level === filter) : sorted;
  return <>
    <Verdict result={result} count={count} />
    {result.local_node ? <div className="banner warn">{t("ttProbe.localNode")}</div> : null}
    <div className="flex flex-wrap gap-2" role="group" aria-live="polite">
      <button type="button" className="chip-btn" aria-pressed={filter === null} onClick={() => setFilter(null)}>{t("ttProbe.filterAll", { n: findings.length })}</button>
      {LEVELS.filter((level) => count(level) > 0).map((level) => <button key={level} type="button" className="chip-btn" aria-pressed={filter === level} onClick={() => setFilter(filter === level ? null : level)}>
        <Pill tone={tone(level)}>{t(`ttProbe.level${level}`)}: {count(level)}</Pill>
      </button>)}
    </div>
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 break-all text-xs">
      <dt>{t("ttProbe.target")}</dt><dd className="mono">{result.report.target}</dd>
      <dt>{t("ttProbe.address")}</dt><dd className="mono">{result.address}</dd>
      <dt>{t("ttProbe.cover")}</dt><dd className="mono">{result.report.reference || t("ttProbe.noReference")}</dd>
      <dt>{t("ttProbe.startedAt")}</dt><dd>{dateLong(result.started_at)} {time(result.started_at)}</dd>
    </dl>
    <Button onClick={onDownload}><Download size={16} aria-hidden />{t("ttProbe.download")}</Button>
    <p className="text-xs text-[var(--ink-500)]">{t("ttProbe.limits")}</p>
    <div className="space-y-2">{shown.map(({ f, i }) => <FindingRow key={i} f={f} />)}</div>
  </>;
}

function Verdict({ result, count }: { result: SingleResult; count: (level: Level) => number }) {
  const [cls, text] = count("FAIL") > 0 ? ["err", t("ttProbe.verdictFAIL", { n: count("FAIL") })]
    : count("ERROR") > 0 ? ["err", t("ttProbe.verdictERROR", { n: count("ERROR") })]
    : !result.report.reference ? ["warn", t("ttProbe.verdictNoReference")]
    : count("WARN") > 0 ? ["warn", t("ttProbe.verdictWARN", { n: count("WARN") })]
    : ["info", t("ttProbe.verdictPASS")];
  return <div role="status" className={`banner ${cls} font-medium`}>{text}</div>;
}

function FindingRow({ f }: { f: Finding }) {
  const bad = f.level === "FAIL" || f.level === "ERROR";
  return <details className="rounded-xl border border-[var(--hairline)] p-3" open={bad || (!!f.compare && f.level === "WARN")}>
    <summary className="cursor-pointer break-words text-sm"><span className={bad ? "text-[var(--berry-600)]" : ""} title={f.level}>{levelLabel(f.level)}</span> · {f.name}</summary>
    {/* The raw JSON of a comparison stays in the downloaded report. */}
    {f.compare ? <FingerprintTable target={f.compare.target} reference={f.compare.reference} />
      : <p className="mono mt-2 whitespace-pre-wrap break-all text-xs text-[var(--ink-500)]">{f.detail}</p>}
  </details>;
}

// HTTP/2 SETTINGS identifiers (RFC 9113, RFC 8441, RFC 9218).
const SETTING_NAMES: Record<number, string> = {
  1: "HEADER_TABLE_SIZE", 2: "ENABLE_PUSH", 3: "MAX_CONCURRENT_STREAMS", 4: "INITIAL_WINDOW_SIZE",
  5: "MAX_FRAME_SIZE", 6: "MAX_HEADER_LIST_SIZE", 8: "ENABLE_CONNECT_PROTOCOL", 9: "NO_RFC7540_PRIORITIES",
};
const settingName = (id: number) => SETTING_NAMES[id] ?? `0x${id.toString(16)}`;

function FingerprintTable({ target, reference }: { target: Fingerprint; reference: Fingerprint }) {
  const value = (fp: Fingerprint, id: number) => fp.settings.find((s) => s.id === id)?.value;
  const ids = [...new Set([...target.settings, ...reference.settings].map((s) => s.id))].sort((a, b) => a - b);
  const missing = t("ttProbe.fpMissing");
  const show = (v: number | undefined) => (v === undefined ? missing : String(v));
  const windows = (fp: Fingerprint) => fp.initial_window_updates.map((w) => `${w.stream}:+${w.increment}`).join(", ") || missing;
  const rows: [string, string, string][] = [
    ...ids.map((id): [string, string, string] => [settingName(id), show(value(target, id)), show(value(reference, id))]),
    [t("ttProbe.fpOrder"), target.settings.map((s) => s.id).join(", "), reference.settings.map((s) => s.id).join(", ")],
    [t("ttProbe.fpWindow"), windows(target), windows(reference)],
    [t("ttProbe.fpHeaders"), target.header_order.join(", "), reference.header_order.join(", ")],
  ];
  const same = rows.every(([, a, b]) => a === b);
  return <div className="mt-2 overflow-x-auto">
    {same ? <p className="text-xs">{t("ttProbe.fpSame")}</p> : null}
    <table className="w-full text-left text-xs">
      <thead><tr><th className="pr-3 font-medium">{t("ttProbe.fpParam")}</th><th className="pr-3 font-medium">{t("ttProbe.fpTarget")}</th><th className="font-medium">{t("ttProbe.fpReference")}</th></tr></thead>
      <tbody>{rows.map(([name, a, b]) => <tr key={name} className={a === b ? "" : "bg-[var(--berry-50)] text-[var(--berry-600)]"}>
        <td className="pr-3 align-top">{name}</td><td className="mono pr-3 align-top">{a}</td><td className="mono align-top">{b}</td>
      </tr>)}</tbody>
    </table>
  </div>;
}

function verdictLabel(verdict: string) {
  switch (verdict) {
    case "quiet": return t("nodeProbe.quiet");
    case "noticeable": return t("nodeProbe.noticeable");
    case "exposed": return t("nodeProbe.exposed");
    default: return t("nodeProbe.inconclusive");
  }
}

function reportReason(report: ProtocolReport) {
  if (report.verdict === "exposed") return report.findings.find((f) => f.level === "FAIL")?.detail;
  if (report.findings.some((f) => f.name === "obfuscation")) return t("nodeProbe.obfuscated");
  if (report.incomplete) return t("nodeProbe.incompleteReason");
  if (!report.reference && report.verdict === "inconclusive") return t("nodeProbe.noReference");
  return report.verdict === "inconclusive" ? t("nodeProbe.insufficient") : undefined;
}
