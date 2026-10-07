import { TriangleAlert } from "lucide-react";
import type { Schemas } from "../../../api/client";
import { Field, Segmented } from "../../../components/ui";
import { Switch } from "../../../components/switch";
import { t } from "../../../i18n";

export type XHTTP = Schemas["XHTTPTuning"];

export const XHTTP_DEFAULT: XHTTP = {
  mode: "",
  x_padding_bytes: "",
  x_padding_obfs_mode: false,
  x_padding_placement: "",
  x_padding_key: "",
  x_padding_header: "",
  x_padding_method: "",
  uplink_http_method: "",
  uplink_chunk_size: "",
  sc_max_each_post_bytes: "",
  xmux_max_concurrency: "",
  xmux_max_connections: "",
  xmux_c_max_reuse_times: "",
  xmux_h_max_request_times: "",
  xmux_h_max_reusable_secs: "",
  sc_min_posts_interval_ms: "",
};

/** A short random token for the padding's key: a fixed one would be a mark of its own. */
function token() {
  const abc = "abcdefghijklmnopqrstuvwxyz";
  const a = new Uint8Array(6);
  crypto.getRandomValues(a);
  return Array.from(a, (x) => abc[x % abc.length]).join("");
}

type Preset = "standard" | "noise" | "max";

// Ready sets: what most admins want without reading the Xray docs. "max" changes how the
// requests look on the wire, so older apps lose the inbound: the form says so.
function preset(p: Preset): XHTTP {
  switch (p) {
    case "noise":
      return { ...XHTTP_DEFAULT, x_padding_bytes: "500-3000", xmux_max_concurrency: "8-16", xmux_h_max_reusable_secs: "900-1800" };
    case "max":
      return {
        ...XHTTP_DEFAULT,
        x_padding_bytes: "1000-4000",
        x_padding_obfs_mode: true,
        x_padding_placement: "cookie",
        x_padding_key: token(),
        x_padding_method: "tokenish",
        uplink_http_method: "PUT",
        xmux_max_concurrency: "4-8",
        xmux_h_max_reusable_secs: "600-1200",
      };
    default:
      return { ...XHTTP_DEFAULT };
  }
}

function presetOf(x: XHTTP): Preset | "own" {
  const same = (a: XHTTP, b: XHTTP) => (Object.keys(a) as (keyof XHTTP)[]).every((k) => k === "x_padding_key" || a[k] === b[k]);
  if (same(x, XHTTP_DEFAULT)) return "standard";
  if (same(x, preset("noise"))) return "noise";
  if (same(x, preset("max"))) return "max";
  return "own";
}

/** The XHTTP tab: the server's settings, which reach the apps, and the apps' own reuse of
 * connections. An empty field is the core's default. */
export function XhttpTuning({ value, onChange, error }: { value: XHTTP; onChange: (x: XHTTP) => void; error?: string }) {
  const set = (k: keyof XHTTP) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => onChange({ ...value, [k]: e.target.value });
  const current = presetOf(value);
  const strong = value.x_padding_obfs_mode || value.uplink_http_method !== "";
  const input = (k: keyof XHTTP, placeholder: string, label: string, hint?: string) => (
    <Field label={label} htmlFor={`x-${k}`} hint={hint}>
      <input id={`x-${k}`} className="input mono" value={value[k] as string} onChange={set(k)} placeholder={placeholder} inputMode="numeric" autoComplete="off" />
    </Field>
  );
  return (
    <div className="flex flex-col gap-1">
      <p className="mb-2 text-[13px] text-[var(--ink-500)]">{t("inbounds.xhttp.intro")}</p>
      <Field label={t("inbounds.xhttp.preset")}>
        <div className="flex flex-wrap gap-2" role="group" aria-label={t("inbounds.xhttp.preset")}>
          {(["standard", "noise", "max"] as Preset[]).map((p) => (
            <button key={p} type="button" className="chip-btn" aria-pressed={current === p} onClick={() => onChange(preset(p))}>
              {t(`inbounds.xhttp.presets.${p}`)}
            </button>
          ))}
          {current === "own" ? <span className="self-center text-xs text-[var(--ink-500)]">{t("inbounds.xhttp.presets.own")}</span> : null}
        </div>
      </Field>
      {strong ? (
        <div className="mb-3 flex items-start gap-2 rounded-xl border border-[var(--hairline)] bg-[var(--honey-50)] p-3 text-xs text-[var(--honey-600)]" role="note">
          <TriangleAlert size={16} className="mt-px shrink-0" aria-hidden />
          <span>{t("inbounds.xhttp.strongWarn")}</span>
        </div>
      ) : null}
      {error ? (
        <p className="mb-3 text-xs text-[var(--berry-600)]" role="alert">
          {error}
        </p>
      ) : null}

      <Field label={t("inbounds.xhttp.mode")} hint={t("inbounds.xhttp.modeHint")}>
        <Segmented
          label={t("inbounds.xhttp.mode")}
          value={value.mode || "default"}
          onChange={(v) => onChange({ ...value, mode: v === "default" ? "" : v })}
          options={[
            { value: "default", label: t("inbounds.xhttp.byDefault") },
            { value: "stream-one", label: "stream-one" },
            { value: "stream-up", label: "stream-up" },
            { value: "packet-up", label: "packet-up" },
          ]}
        />
      </Field>

      <h3 className="mt-3 mb-1 text-[13px] font-semibold">{t("inbounds.xhttp.padding")}</h3>
      {input("x_padding_bytes", "100-1000", t("inbounds.xhttp.paddingBytes"), t("inbounds.xhttp.rangeHint"))}
      <div className="flex items-start justify-between gap-4 py-2">
        <div className="min-w-0">
          <div className="text-[13px] font-medium">{t("inbounds.xhttp.obfs")}</div>
          <div className="mt-1 text-xs text-[var(--ink-500)]">{t("inbounds.xhttp.obfsHint")}</div>
        </div>
        <Switch
          checked={!!value.x_padding_obfs_mode}
          label={t("inbounds.xhttp.obfs")}
          onChange={(v) => onChange({ ...value, x_padding_obfs_mode: v, x_padding_key: v && !value.x_padding_key ? token() : value.x_padding_key })}
        />
      </div>
      {value.x_padding_obfs_mode ? (
        <div className="grid gap-x-3 sm:grid-cols-2">
          <Field label={t("inbounds.xhttp.placement")} htmlFor="x-placement">
            <select id="x-placement" className="input" value={value.x_padding_placement} onChange={set("x_padding_placement")}>
              <option value="">{t("inbounds.xhttp.byDefault")}</option>
              <option value="cookie">cookie</option>
              <option value="header">header</option>
              <option value="query">query</option>
              <option value="queryInHeader">queryInHeader</option>
            </select>
          </Field>
          <Field label={t("inbounds.xhttp.method")} htmlFor="x-method">
            <select id="x-method" className="input" value={value.x_padding_method} onChange={set("x_padding_method")}>
              <option value="">{t("inbounds.xhttp.byDefault")}</option>
              <option value="repeat-x">repeat-x</option>
              <option value="tokenish">tokenish</option>
            </select>
          </Field>
          {input("x_padding_key", "x_padding", t("inbounds.xhttp.key"))}
          {input("x_padding_header", "Referer", t("inbounds.xhttp.header"))}
        </div>
      ) : null}

      <h3 className="mt-3 mb-1 text-[13px] font-semibold">{t("inbounds.xhttp.upload")}</h3>
      <div className="grid gap-x-3 sm:grid-cols-3">
        <Field label={t("inbounds.xhttp.uplinkMethod")} htmlFor="x-uplink">
          <select id="x-uplink" className="input" value={value.uplink_http_method} onChange={set("uplink_http_method")}>
            <option value="">POST</option>
            <option value="PUT">PUT</option>
            <option value="PATCH">PATCH</option>
          </select>
        </Field>
        {input("uplink_chunk_size", "", t("inbounds.xhttp.chunk"))}
        {input("sc_max_each_post_bytes", "1000000", t("inbounds.xhttp.maxPost"))}
      </div>

      <h3 className="mt-3 mb-1 text-[13px] font-semibold">{t("inbounds.xhttp.reuse")}</h3>
      <p className="mb-2 text-xs text-[var(--ink-500)]">{t("inbounds.xhttp.reuseHint")}</p>
      <div className="grid gap-x-3 sm:grid-cols-2">
        {input("xmux_max_concurrency", "16-32", t("inbounds.xhttp.concurrency"))}
        {input("xmux_max_connections", "", t("inbounds.xhttp.connections"), t("inbounds.xhttp.connectionsHint"))}
        {input("xmux_c_max_reuse_times", "", t("inbounds.xhttp.reuseTimes"))}
        {input("xmux_h_max_request_times", "600-900", t("inbounds.xhttp.requestTimes"))}
        {input("xmux_h_max_reusable_secs", "1800-3000", t("inbounds.xhttp.reusableSecs"))}
        {input("sc_min_posts_interval_ms", "30", t("inbounds.xhttp.postInterval"))}
      </div>
    </div>
  );
}
