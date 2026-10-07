import type { FormEvent } from "react";
import type { Schemas } from "../../../api/client";
import { Button, Field, Segmented } from "../../../components/ui";
import { Switch } from "../../../components/switch";
import { t } from "../../../i18n";
import { useDraft } from "../../../lib/draft";
import { fieldErrors } from "../../../lib/fields";
import { useSaveSettings } from "./shared";

type Crypt = "off" | "api" | "local";

/** What Happ gets beyond the profile: a routing profile, hidden server settings and a
 * crypt link that keeps the subscription address out of sight. */
export function HappCard({ s }: { s: Schemas["SettingsView"] }) {
  const save = useSaveSettings();
  const { draft: form, setDraft: setForm } = useDraft({
    happ_routing: s.happ_routing,
    happ_provider_id: s.happ_provider_id,
    happ_crypt: (s.happ_crypt || "off") as Crypt,
  });
  const errors = fieldErrors(save.error);
  const provider = form.happ_provider_id.trim();
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate({ happ_routing: form.happ_routing.trim(), happ_provider_id: provider, happ_crypt: form.happ_crypt });
  };
  // The switch saves at once, like the other switches; it needs the saved provider id.
  const canHide = !!s.happ_provider_id;
  return (
    <section className="card glass reveal" style={{ "--i": 3 } as React.CSSProperties}>
      <form onSubmit={submit} noValidate>
        <div className="card-head">
          <div>
            <h2 className="card-title">Happ</h2>
            <div className="card-sub">{t("settings.happSub")}</div>
          </div>
        </div>

        <Field label={t("settings.happCrypt")} hint={t(`settings.happCryptHint.${form.happ_crypt}`)} error={errors.happ_crypt}>
          <Segmented
            label={t("settings.happCrypt")}
            value={form.happ_crypt}
            onChange={(v) => setForm((f) => ({ ...f, happ_crypt: v }))}
            options={[
              { value: "off", label: t("settings.happCryptOff") },
              { value: "api", label: t("settings.happCryptApi") },
              { value: "local", label: t("settings.happCryptLocal") },
            ]}
          />
        </Field>

        <Field label={t("settings.happRouting")} htmlFor="s-happ-routing" hint={t("settings.happRoutingHint")} error={errors.happ_routing}>
          <textarea
            id="s-happ-routing"
            className="input mono min-h-[88px] text-xs"
            value={form.happ_routing}
            onChange={(e) => setForm((f) => ({ ...f, happ_routing: e.target.value }))}
            placeholder="happ://routing/onadd/eyJOYW1lIjoi…"
            spellCheck={false}
            autoComplete="off"
            aria-invalid={!!errors.happ_routing}
          />
        </Field>

        <Field label={t("settings.happProvider")} htmlFor="s-happ-provider" hint={t("settings.happProviderHint")} error={errors.happ_provider_id}>
          <input
            id="s-happ-provider"
            className="input mono"
            value={form.happ_provider_id}
            onChange={(e) => setForm((f) => ({ ...f, happ_provider_id: e.target.value }))}
            maxLength={64}
            autoComplete="off"
            aria-invalid={!!errors.happ_provider_id}
          />
        </Field>

        <div className="flex items-start justify-between gap-4 py-3">
          <div className="min-w-0">
            <div className="text-[13px] font-medium">{t("settings.happHide")}</div>
            <div className={canHide ? "mt-1 text-xs text-[var(--ink-500)]" : "mt-1 text-xs text-[var(--honey-600)]"}>
              {canHide ? t("settings.happHideSub") : t("settings.happHideNeedsProvider")}
            </div>
            {errors.happ_hide_settings ? (
              <div className="mt-1 text-xs text-[var(--berry-600)]" role="alert">
                {errors.happ_hide_settings}
              </div>
            ) : null}
          </div>
          <Switch
            checked={canHide && s.happ_hide_settings}
            label={t("settings.happHide")}
            disabled={!canHide || save.isPending}
            onChange={(v) => save.mutate({ happ_hide_settings: v })}
          />
        </div>

        <Button type="submit" variant="primary" loading={save.isPending}>
          {t("common.save")}
        </Button>
      </form>
    </section>
  );
}
