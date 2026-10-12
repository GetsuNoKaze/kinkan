import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorText, unwrap, type Schemas } from "../../api/client";
import { Button, Field } from "../../components/ui";
import { t } from "../../i18n";

type Policy = Schemas["LeakPolicy"];

export function KinkanUserLeaks({ userID }: { userID: number }) {
  const events = useQuery({ queryKey: ["kinkan-leaks", userID], queryFn: () => unwrap(api.GET("/api/v1/users/{id}/leak-events", { params: { path: { id: userID } } })), refetchInterval: 30_000 });
  const policy = useQuery({ queryKey: ["kinkan-leak-policy"], queryFn: () => unwrap(api.GET("/api/v1/kinkan/leak-policy")) });
  return <section className="space-y-3 py-3">
    <h3>{t("leaks.title")}</h3>
    <p className="text-sm text-[var(--ink-500)]">{t("leaks.explanation")}</p>
    {events.isPending ? <p>{t("leaks.loading")}</p> : events.isError ? <div className="banner warn">{errorText(events.error)}</div> : events.data?.events.length ? <div className="banner warn">
      <strong>{t("leaks.warning")}</strong>
      {events.data.events.slice(0, 5).map(e => <p key={e.id} className="text-sm">{new Date(e.last_seen * 1000).toLocaleString()} · {t("leaks.evidence", { networks: e.networks.join(", ") || "—", countries: e.countries.join(", ") || "—", minutes: e.window_minutes })}</p>)}
    </div> : <p className="text-sm">{t("leaks.none")}</p>}
    <details><summary className="cursor-pointer text-sm">{t("leaks.settings")}</summary>
      {policy.isError ? <div className="banner warn">{errorText(policy.error)}</div> : policy.data ? <LeakSettings key={JSON.stringify(policy.data)} policy={policy.data} /> : <p>{t("leaks.loading")}</p>}
    </details>
  </section>;
}

function LeakSettings({ policy }: { policy: Policy }) {
  const [value, setValue] = useState(policy);
  const qc = useQueryClient();
  const save = useMutation({ mutationFn: () => unwrap(api.PUT("/api/v1/kinkan/leak-policy", { body: value })), onSuccess: () => { void qc.invalidateQueries({ queryKey: ["kinkan-leak-policy"] }); } });
  return <form className="space-y-2 pt-3" onSubmit={e => { e.preventDefault(); save.mutate(); }}>
    <p className="text-sm">{t("leaks.global")}</p>
    <label><input type="checkbox" checked={value.enabled} onChange={e => setValue({ ...value, enabled: e.target.checked })} /> {t("leaks.enabled")}</label>
    <Field label={t("leaks.networks")}><input type="number" min={2} max={100} required value={value.networks} onChange={e => setValue({ ...value, networks: Number(e.target.value) })} /></Field>
    <Field label={t("leaks.countries")}><input type="number" min={2} max={100} required value={value.countries} onChange={e => setValue({ ...value, countries: Number(e.target.value) })} /></Field>
    <Field label={t("leaks.window")}><input type="number" min={1} max={60} required value={value.window_minutes} onChange={e => setValue({ ...value, window_minutes: Number(e.target.value) })} /></Field>
    {save.isError ? <p role="alert">{errorText(save.error)}</p> : null}
    {save.isSuccess ? <p role="status">{t("leaks.saved")}</p> : null}
    <Button type="submit" loading={save.isPending}>{t("leaks.save")}</Button>
  </form>;
}
