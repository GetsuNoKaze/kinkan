import { useMutation, useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, ApiError, errorText, unwrap, type Schemas } from "../../../api/client";
import { useNodes } from "../../../api/hooks";
import { Columns } from "../../../components/tabs";
import { Button, ErrorState, Field, Pill, Skeleton } from "../../../components/ui";
import { t, useLocale } from "../../../i18n";
import { useDraft } from "../../../lib/draft";
import { useSaveSettings } from "./shared";
import { TemplateCard } from "./template";

type Routes = Schemas["Routes"];
type DNS = NonNullable<Routes["dns"]>;

const MODES = [
  { id: "ru_direct", title: "settings.routingRuDirect", sub: "settings.routingRuDirectSub" },
  { id: "all", title: "settings.routingAll", sub: "settings.routingAllSub" },
  { id: "blocked", title: "settings.routingBlocked", sub: "settings.routingBlockedSub" },
] as const;

// Where a service can go; "node:<id>" options follow from the nodes.
const TARGETS = [
  { id: "", label: "settings.routesFollow" },
  { id: "vpn", label: "settings.routesVpn" },
  { id: "direct", label: "settings.routesDirect" },
  { id: "block", label: "settings.routesBlock" },
] as const;

// Ready DNS sets for the advanced part (GitHub issue #70): the panel's own is the empty one.
const DNS_PRESETS: { id: string; dns: DNS }[] = [
  { id: "panel", dns: {} },
  { id: "tunnel", dns: { nameserver: ["https://1.1.1.1/dns-query#PROXY", "https://8.8.8.8/dns-query#PROXY"], proxy_server_nameserver: ["1.1.1.1", "8.8.8.8"] } },
  {
    id: "ruYandex",
    dns: {
      nameserver: ["https://1.1.1.1/dns-query#PROXY", "https://8.8.8.8/dns-query#PROXY"],
      proxy_server_nameserver: ["77.88.8.8", "1.1.1.1"],
      policy: [
        { match: "+.ru", servers: ["https://77.88.8.8/dns-query"] },
        { match: "+.su", servers: ["https://77.88.8.8/dns-query"] },
        { match: "+.xn--p1ai", servers: ["https://77.88.8.8/dns-query"] },
      ],
    },
  },
];

const lines = (s: string) =>
  s
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);

// "+.cn: https://dns.alidns.com/dns-query, 223.5.5.5" per line.
function parsePolicy(text: string): NonNullable<DNS["policy"]> {
  return lines(text).map((l) => {
    const [match = "", rest = ""] = l.split(/:\s+/, 2);
    return { match: match.trim(), servers: rest.split(",").map((x) => x.trim()).filter(Boolean) };
  });
}
const policyText = (p: DNS["policy"]) => (p ?? []).map((x) => `${x.match}: ${x.servers.join(", ")}`).join("\n");

/** The routing of Clash profiles: the mode, the services, the apps past the tunnel and DNS, with the profile it makes. */
export function RoutingSection({ s }: { s: Schemas["SettingsView"] }) {
  const save = useSaveSettings();
  const nodes = useNodes();
  const locale = useLocale();
  const catalog = useQuery({ queryKey: ["routes-catalog"], queryFn: ({ signal }) => unwrap(api.GET("/api/v1/settings/routes/catalog", { signal })), staleTime: Infinity });
  const { draft, setDraft, dirty, reset } = useDraft({ mode: s.sub_routing, routes: s.sub_routes });
  const routes = draft.routes;
  const setRoutes = (f: (r: Routes) => Routes) => setDraft((d) => ({ ...d, routes: f(d.routes) }));
  const [dnsText, setDnsText] = useState(() => ({
    nameserver: (routes.dns?.nameserver ?? []).join("\n"),
    proxy: (routes.dns?.proxy_server_nameserver ?? []).join("\n"),
    policy: policyText(routes.dns?.policy),
  }));
  const setDns = (dns: DNS) => {
    setRoutes((r) => ({ ...r, dns }));
    setDnsText({ nameserver: (dns.nameserver ?? []).join("\n"), proxy: (dns.proxy_server_nameserver ?? []).join("\n"), policy: policyText(dns.policy) });
  };
  const editDns = (k: keyof typeof dnsText) => (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const next = { ...dnsText, [k]: e.target.value };
    setDnsText(next);
    setRoutes((r) => ({ ...r, dns: { nameserver: lines(next.nameserver), proxy_server_nameserver: lines(next.proxy), policy: parsePolicy(next.policy) } }));
  };
  const setService = (id: string, target: string) =>
    setRoutes((r) => {
      const services = { ...(r.services ?? {}) };
      if (target) services[id] = target;
      else delete services[id];
      return { ...r, services };
    });
  const toggleDirect = (id: string) =>
    setRoutes((r) => {
      const has = (r.direct ?? []).includes(id);
      return { ...r, direct: has ? (r.direct ?? []).filter((x) => x !== id) : [...(r.direct ?? []), id] };
    });
  const apiErr = save.error instanceof ApiError ? save.error : null;
  const error = apiErr?.fields.sub_routes;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate({ sub_routing: draft.mode, sub_routes: draft.routes });
  };
  const nodeName = (n: { name: string; local: boolean }) => t("settings.routesOnServer", { name: n.name || (n.local ? t("settings.routesThisServer") : "—") });
  const routed = Object.keys(routes.services ?? {}).length + (routes.direct ?? []).length;
  // The own profile opens where it is in use; its text lives here, so switching views keeps it.
  const [view, setView] = useState<"simple" | "yaml">(s.sub_template.trim() ? "yaml" : "simple");
  const [yaml, setYaml] = useState(s.sub_template);
  const starter = () => unwrap(api.POST("/api/v1/settings/routes/preview", { body: { sub_routing: draft.mode, sub_routes: draft.routes, starter: true } })).then((r) => r.profile);
  const views = (
    <div className="mb-4 flex flex-wrap gap-2" role="group" aria-label={t("settings.routesView")}>
      {(["simple", "yaml"] as const).map((v) => (
        <button key={v} type="button" aria-pressed={view === v} className="chip" onClick={() => setView(v)}>
          {t(v === "simple" ? "settings.routesViewSimple" : "settings.routesViewYaml")}
        </button>
      ))}
    </div>
  );
  if (view === "yaml") {
    return (
      <>
        {views}
        <Columns wide="left" left={<TemplateCard s={s} text={yaml} setText={setYaml} starter={starter} />} right={<PreviewCard mode={draft.mode} routes={draft.routes} template={yaml} />} />
      </>
    );
  }
  return (
    <>
    {views}
    {s.sub_template.trim() ? <div className="banner mb-4">{t("settings.routesYamlLive")}</div> : null}
    <Columns
      wide="left"
      left={
        <section className="card glass reveal" style={{ "--i": 1 } as React.CSSProperties}>
          <form onSubmit={submit} noValidate>
            <div className="card-head">
              <div>
                <h2 className="card-title">{t("settings.routes")}</h2>
                <div className="card-sub">{t("settings.routesSub")}</div>
              </div>
              {routed ? <Pill tone="off">{t("settings.routesCount", { n: routed })}</Pill> : null}
            </div>

            <Field label={t("settings.routing")} hint={t("settings.routingHint")}>
              <div className="grid gap-2 sm:grid-cols-3" role="radiogroup" aria-label={t("settings.routing")}>
                {MODES.map((m) => (
                  <button key={m.id} type="button" role="radio" aria-checked={draft.mode === m.id} className="opt" onClick={() => setDraft((d) => ({ ...d, mode: m.id }))}>
                    <span className="font-semibold">{t(m.title)}</span>
                    <span className="text-xs text-[var(--ink-500)]">{t(m.sub)}</span>
                  </button>
                ))}
              </div>
            </Field>

            <Field label={t("settings.routesServices")} hint={t("settings.routesServicesHint")}>
              {catalog.isError ? <ErrorState text={errorText(catalog.error)} onRetry={() => void catalog.refetch()} /> : null}
              {catalog.isPending ? (
                <div className="flex flex-col gap-2" aria-busy="true">
                  {[0, 1, 2, 3].map((i) => (
                    <Skeleton key={i} className="h-9 w-full rounded-xl" />
                  ))}
                </div>
              ) : null}
              <div className="flex flex-col divide-y divide-[var(--hairline)]">
                {(catalog.data?.services ?? []).map((svc) => {
                  const id = `s-svc-${svc.id}`;
                  return (
                    <div key={svc.id} className="flex flex-wrap items-center justify-between gap-2 py-2">
                      <label htmlFor={id} className="flex min-w-0 items-center gap-2 text-sm">
                        <span aria-hidden>{svc.icon}</span>
                        <span className="truncate">{locale === "en" ? svc.name_en : svc.name}</span>
                      </label>
                      <select id={id} className="input max-w-[220px]" value={routes.services?.[svc.id] ?? ""} onChange={(e) => setService(svc.id, e.target.value)}>
                        {TARGETS.map((x) => (
                          <option key={x.id} value={x.id}>
                            {t(x.label)}
                          </option>
                        ))}
                        {(nodes.data ?? []).length ? (
                          <optgroup label={t("settings.routesServer")}>
                            {(nodes.data ?? []).map((n) => (
                              <option key={n.id} value={`node:${n.id}`}>
                                {nodeName(n)}
                              </option>
                            ))}
                          </optgroup>
                        ) : null}
                      </select>
                    </div>
                  );
                })}
              </div>
            </Field>

            <Field label={t("settings.routesApps")} hint={t("settings.routesAppsHint")}>
              <div className="flex flex-col gap-2">
                {(catalog.data?.direct ?? []).map((d) => (
                  <label key={d.id} className="flex items-start gap-2 text-sm">
                    <input type="checkbox" className="mt-1" checked={(routes.direct ?? []).includes(d.id)} onChange={() => toggleDirect(d.id)} />
                    <span>{locale === "en" ? d.name_en : d.name}</span>
                  </label>
                ))}
              </div>
            </Field>

            <details className="mb-4">
              <summary className="cursor-pointer text-sm font-medium text-[var(--ink-700)]">{t("settings.routesDns")}</summary>
              <p className="mt-2 text-xs text-[var(--ink-500)]">{t("settings.routesDnsHint")}</p>
              <div className="my-3 flex flex-wrap gap-2" role="group" aria-label={t("settings.routesDnsPresets")}>
                {DNS_PRESETS.map((p) => (
                  <button key={p.id} type="button" className="chip-btn" onClick={() => setDns(p.dns)}>
                    {t(`settings.routesDnsPreset.${p.id}` as "settings.routesDnsPreset.panel")}
                  </button>
                ))}
              </div>
              <Field label={t("settings.routesDnsServers")} htmlFor="s-dns-ns" hint={t("settings.routesDnsServersHint")}>
                <textarea id="s-dns-ns" className="input mono min-h-[72px]" value={dnsText.nameserver} onChange={editDns("nameserver")} spellCheck={false} placeholder="https://1.1.1.1/dns-query#PROXY" />
              </Field>
              <Field label={t("settings.routesDnsProxy")} htmlFor="s-dns-proxy" hint={t("settings.routesDnsProxyHint")}>
                <textarea id="s-dns-proxy" className="input mono min-h-[56px]" value={dnsText.proxy} onChange={editDns("proxy")} spellCheck={false} placeholder="1.1.1.1" />
              </Field>
              <Field label={t("settings.routesDnsPolicy")} htmlFor="s-dns-policy" hint={t("settings.routesDnsPolicyHint")}>
                <textarea id="s-dns-policy" className="input mono min-h-[72px]" value={dnsText.policy} onChange={editDns("policy")} spellCheck={false} placeholder="+.cn: https://dns.alidns.com/dns-query" />
              </Field>
            </details>

            {error ? (
              <p className="mb-3 text-xs text-[var(--berry-600)]" role="alert">
                {error}
              </p>
            ) : null}
            <p className="mb-4 text-xs text-[var(--ink-500)]">{t("settings.routesNote")}</p>
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary" loading={save.isPending} disabled={!dirty}>
                {t("common.save")}
              </Button>
              {dirty ? (
                <Button variant="ghost" onClick={reset}>
                  {t("telegram.discard")}
                </Button>
              ) : null}
            </div>
          </form>
        </section>
      }
      right={<PreviewCard mode={draft.mode} routes={draft.routes} />}
    />
    </>
  );
}

// The profile a user with every connection gets, before anything is saved.
function PreviewCard({ mode, routes, template }: { mode: Schemas["SettingsView"]["sub_routing"]; routes: Routes; template?: string }) {
  const preview = useMutation({
    mutationFn: () => unwrap(api.POST("/api/v1/settings/routes/preview", { body: { sub_routing: mode, sub_routes: routes, ...(template?.trim() ? { sub_template: template } : {}) } })),
  });
  return (
    <section className="card glass reveal" style={{ "--i": 2 } as React.CSSProperties}>
      <div className="card-head">
        <div>
          <h2 className="card-title">{t("settings.routesPreview")}</h2>
          <div className="card-sub">{t("settings.routesPreviewSub")}</div>
        </div>
      </div>
      <Button variant="ghost" loading={preview.isPending} onClick={() => preview.mutate()}>
        {preview.data ? t("settings.routesPreviewAgain") : t("settings.routesPreviewShow")}
      </Button>
      {preview.error ? (
        <p className="mt-3 text-xs text-[var(--berry-600)]" role="alert">
          {errorText(preview.error)}
        </p>
      ) : null}
      {preview.data ? <pre className="panel-soft mono mt-3 max-h-[70vh] overflow-auto p-3 text-[12px] leading-[1.45]">{preview.data.profile}</pre> : null}
    </section>
  );
}
