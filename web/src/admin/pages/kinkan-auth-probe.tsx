import { t } from "../../i18n";

type Observation = { kind: string; bytes: number; status?: number; sha256?: string; elapsed_ms: number; close: string; detail?: string };
type Comparison = { verdict: "site" | "different" | "exposed" | "inconclusive"; reason: string; target: Observation; reference?: Observation };

// Accept unknown for old, saved reports and the historical TrustTunnel report shape.
export function KinkanAuthProbe({ report }: { report: unknown }) {
  const comparison = (report as { wrong_key?: Comparison }).wrong_key;
  if (!comparison) return null;
  return <div className={`banner ${comparison.verdict === "site" ? "info" : "warn"}`}>
    <strong>{t(`wrongKey.${comparison.verdict}`)}</strong>
    <p>{comparison.reason}</p>
    <table className="w-full text-left text-sm"><thead><tr><th>{t("wrongKey.endpoint")}</th><th>{t("wrongKey.response")}</th><th>{t("wrongKey.close")}</th><th>{t("wrongKey.elapsed")}</th></tr></thead>
      <tbody>{([["target", comparison.target], ["reference", comparison.reference]] as const).map(([name, value]) => {
        const o = value as Observation | undefined;
        return o ? <tr key={String(name)}><td>{t(`wrongKey.${name}`)}</td><td>{o.kind}{o.status ? ` · HTTP ${o.status}` : ""} · {o.bytes ?? 0} B</td><td>{o.close}</td><td>{o.elapsed_ms} ms</td></tr> : null;
      })}</tbody>
    </table>
  </div>;
}
