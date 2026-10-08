import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Bot, ChevronRight, ListFilter, ShieldBan, Wallet, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { api, unwrap } from "../../api/client";
import { qk, useFilters, useTorrent } from "../../api/hooks";
import { QueryBoundary } from "../../components/query";
import { PageHeader, Pill, Skeleton } from "../../components/ui";
import { t } from "../../i18n";
import { addonName, useAddons } from "./payment-addons";
import { TorrentCard, TorrentHitsCard } from "./settings/torrent";

/**
 * The addons: the tools built into the panel (the Telegram bot, the torrent blocker) and
 * the marketplace's payment methods, each with its state and the way to its settings. The
 * main menu keeps the everyday sections; what is switched on once lives here.
 */
export function AddonsPage() {
  return (
    <>
      <PageHeader title={t("nav.addons")} sub={t("addons.subtitle")} />
      <h2 className="section-title">{t("addons.tools")}</h2>
      <div className="addon-grid">
        <TelegramTile />
        <TorrentTile />
        <FiltersTile />
      </div>
      <h2 className="section-title mt-6">{t("addons.payments")}</h2>
      <PaymentTiles />
    </>
  );
}

function Tile({ to, search, icon: Icon, title, sub, state, i }: { to: string; search?: Record<string, string>; icon: LucideIcon; title: string; sub: string; state: ReactNode; i: number }) {
  return (
    <Link to={to} search={search} className="card glass reveal addon-tile" style={{ "--i": i } as React.CSSProperties}>
      <span className="addon-icon" aria-hidden>
        <Icon size={20} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{title}</span>
          {state}
        </span>
        <span className="mt-1 block text-xs text-[var(--ink-500)]">{sub}</span>
      </span>
      <ChevronRight size={18} className="shrink-0 text-[var(--ink-400)]" aria-hidden />
    </Link>
  );
}

function TelegramTile() {
  const tg = useQuery({ queryKey: qk.telegram, queryFn: ({ signal }) => unwrap(api.GET("/api/v1/telegram", { signal })) });
  const v = tg.data;
  const state = !v ? null : v.running ? <Pill tone="ok">{t("addons.on")}</Pill> : v.enabled ? <Pill tone="warn">{t("addons.stopped")}</Pill> : <Pill tone="off">{t("addons.off")}</Pill>;
  return <Tile to="/addons/telegram" icon={Bot} title={t("addons.telegram")} sub={t("addons.telegramSub")} state={state} i={1} />;
}

function TorrentTile() {
  const q = useTorrent();
  const state = !q.data ? null : q.data.enabled ? <Pill tone="ok">{t("addons.on")}</Pill> : <Pill tone="off">{t("addons.off")}</Pill>;
  return <Tile to="/addons/torrent" icon={ShieldBan} title={t("settings.torrent.title")} sub={t("addons.torrentSub")} state={state} i={2} />;
}

function FiltersTile() {
  const q = useFilters();
  const on = q.data ? q.data.egress.enabled || q.data.ingress.enabled : undefined;
  const state = on === undefined ? null : on ? <Pill tone="ok">{t("addons.on")}</Pill> : <Pill tone="off">{t("addons.off")}</Pill>;
  return <Tile to="/addons/filters" icon={ListFilter} title={t("filters.title")} sub={t("addons.filtersSub")} state={state} i={3} />;
}

// The payment methods of the marketplace: installed ones with their state, and the way to
// install and set them up, which stays in Payments.
function PaymentTiles() {
  const q = useAddons();
  return (
    <QueryBoundary query={q} pending={<Skeleton style={{ height: 72, borderRadius: 20 }} />} wrap={(state) => <section className="card glass">{state}</section>}>
      {(d) => (
        <div className="addon-grid">
          {d.installed.map((a, i) => (
            <Tile
              key={a.id}
              to="/payments"
              search={{ tab: "methods" }}
              icon={Wallet}
              title={addonName(a.id, d)}
              sub={t("addons.paymentSub")}
              state={a.status === "failed" ? <Pill tone="bad">{t("addons.stateFailed")}</Pill> : a.enabled ? <Pill tone="ok">{t("addons.on")}</Pill> : <Pill tone="off">{t("addons.off")}</Pill>}
              i={4 + i}
            />
          ))}
          <Tile to="/payments" search={{ tab: "methods" }} icon={Wallet} title={t("addons.paymentAdd")} sub={t("addons.paymentAddSub", { n: d.catalog.length })} state={null} i={4 + d.installed.length} />
        </div>
      )}
    </QueryBoundary>
  );
}

/** The torrent blocker, an addon of its own: its switch and its catches. */
export function TorrentPage() {
  return (
    <>
      <PageHeader title={t("settings.torrent.title")} sub={t("addons.torrentSub")} actions={<BackToAddons />} />
      <div className="flex max-w-4xl flex-col gap-4">
        <TorrentCard />
        <TorrentHitsCard />
      </div>
    </>
  );
}

export function BackToAddons() {
  return (
    <Link to="/addons" className="btn btn-glass">
      <ArrowLeft size={16} aria-hidden />
      {t("addons.back")}
    </Link>
  );
}
