import { useEffect, useState } from "react";
import type { Server } from "../protokol";
import type { PodpiskaNaEkrane, ZamerZaderzhki } from "./Servery";
import { vozrast } from "./Servery";
import { slovoPosleChisla } from "../chisla";
import { IkGalka, IkServer, IkStrelkaVniz, IkObnovit, IkZakrepit } from "../ikonki";

interface Gruppa { id: string; imya: string; servery: Server[]; podpiska?: PodpiskaNaEkrane }
export function gruppyServerov(servery: Server[], podpiski: PodpiskaNaEkrane[], uzel = ""): Gruppa[] {
  const aktualnye = servery.filter(s => !s.uderzhan);
  const gruppy: Gruppa[] = podpiski.map(p => ({ id: p.id, imya: p.imya || p.uzel, podpiska: p,
    servery: (p.aktivnaya ? aktualnye.filter(s => s.iz_podpiski) : p.servery ?? []).filter(s => !s.uderzhan) }));
  if (!podpiski.length && aktualnye.some(s => s.iz_podpiski)) {
    gruppy.push({ id: "podpiska", imya: uzel || "Подписка", servery: aktualnye.filter(s => s.iz_podpiski) });
  }
  // Ключи своего сервера полосой по адресу, а не среди ручных: их поставил
  // раздел «Свой VPN», и откуда они, видно по заголовку (план 2026-11-01, §4.1).
  const svoi = aktualnye.filter(s => !s.iz_podpiski && s.svoy);
  for (const host of new Set(svoi.map(s => s.host))) {
    gruppy.push({ id: `svoy:${host}`, imya: `Свой сервер ${host}`, servery: svoi.filter(s => s.host === host) });
  }
  const ruchnye = aktualnye.filter(s => !s.iz_podpiski && !s.svoy);
  if (ruchnye.length) gruppy.push({ id: "ruchnye", imya: "Добавлены вручную", servery: ruchnye });
  return gruppy;
}

interface Vid { zakrepleny: string[]; skryty: string[]; svernuty: string[] }
const KLYUCH = "affory.server-view.v1";
const pustoy: Vid = { zakrepleny: [], skryty: [], svernuty: [] };
function stroki(v: unknown): v is string[] { return Array.isArray(v) && v.every(s => typeof s === "string"); }
function chitatVid(): { vid: Vid; otkaz: string } {
  try {
    const raw = localStorage.getItem(KLYUCH);
    if (!raw) return { vid: pustoy, otkaz: "" };
    const v: unknown = JSON.parse(raw);
    if (typeof v === "object" && v !== null && "zakrepleny" in v && "skryty" in v && "svernuty" in v && stroki(v.zakrepleny) && stroki(v.skryty) && stroki(v.svernuty)) {
      return { vid: { zakrepleny: v.zakrepleny, skryty: v.skryty, svernuty: v.svernuty }, otkaz: "" };
    }
    return { vid: pustoy, otkaz: "Настройки списка повреждены. Настрой отображение заново." };
  } catch {
    return { vid: pustoy, otkaz: "Не удалось прочитать настройки списка." };
  }
}

const SMENA_ID = "affory:server-ids";
export function perenestiIdServerov(podpiska: string, ids: Record<string, string>) {
  if (!Object.keys(ids).length) return;
  const state = chitatVid();
  const prefix = `${podpiska}:`;
  const vid = { ...state.vid, zakrepleny: [...new Set(state.vid.zakrepleny.map(key => {
    const id = key.startsWith(prefix) ? ids[key.slice(prefix.length)] : undefined;
    return id ? `${prefix}${id}` : key;
  }))] };
  let otkaz = state.otkaz;
  try { localStorage.setItem(KLYUCH, JSON.stringify(vid)); }
  catch { otkaz = "Не удалось сохранить закрепления после обновления идентификаторов."; }
  window.dispatchEvent(new CustomEvent(SMENA_ID, { detail: { vid, otkaz } }));
}

interface Props {
  servery: Server[]; podpiski: PodpiskaNaEkrane[]; uzel?: string; zapros: string;
  zaderzhki: ZamerZaderzhki[]; nesushchiy?: string; vybran?: string; podnyat: boolean; disabled: boolean;
  naVybor?: (id: string, podpiska?: string) => void;
  naObnovit?: (id: string) => void; obnovlyaetsya?: boolean; obnovlyaemyePodpiski?: string[];
}

export function KatalogServerov({ servery, podpiski, uzel, zapros, zaderzhki, nesushchiy, vybran, podnyat, disabled, naVybor, naObnovit, obnovlyaetsya, obnovlyaemyePodpiski = [] }: Props) {
  const [state, setState] = useState(chitatVid);
  useEffect(() => {
    const obnovit = (event: Event) => setState((event as CustomEvent<{ vid: Vid; otkaz: string }>).detail);
    window.addEventListener(SMENA_ID, obnovit);
    return () => window.removeEventListener(SMENA_ID, obnovit);
  }, []);
  const [pokazatSkrytye, setPokazatSkrytye] = useState(false);
  const { vid } = state;
  const gruppy = gruppyServerov(servery, podpiski, uzel);
  const z = zapros.trim().toLowerCase();
  const strokiGrupp = gruppy.flatMap(g => g.servery.map(s => ({ s, g, key: `${g.id}:${s.id}` })));
  const sovpadaet = ({ s, g }: typeof strokiGrupp[number]) => `${s.imya} ${s.host} ${s.transport} ${g.imya}`.toLowerCase().includes(z);
  const zakreplennye = vid.zakrepleny.flatMap(key => strokiGrupp.filter(r => r.key === key && sovpadaet(r)));
  const skrytye = gruppy.filter(g => vid.skryty.includes(g.id));
  const zamerPoId = new Map(zaderzhki.map(m => [m.id, m]));
  const pomenyat = (pole: keyof Vid, key: string) => {
    const next = { ...vid, [pole]: vid[pole].includes(key) ? vid[pole].filter(k => k !== key) : [...vid[pole], key] };
    let otkaz = "";
    try { localStorage.setItem(KLYUCH, JSON.stringify(next)); }
    catch { otkaz = "Не удалось сохранить настройки списка. Изменения действуют до закрытия окна."; }
    setState({ vid: next, otkaz });
  };
  const ryad = (r: typeof strokiGrupp[number], zakreplen = false) => {
    const { s, g, key } = r;
    const tekushchaya = !g.podpiska || g.podpiska.aktivnaya;
    const active = tekushchaya && podnyat && nesushchiy === s.id;
    const selected = tekushchaya && !podnyat && vybran === s.id;
    // Замер другой подписки с таким же адресным ID не относится к её ключам.
    const m = tekushchaya ? zamerPoId.get(s.id) : undefined;
    // Пинг одним числом, как в Discord (01.10.2026). Причина отказа в подсказке,
    // короткая от ядра в самой строке (02.10.2026); служба пишет её с маленькой.
    const prichina = m?.ping_prichina ? m.ping_prichina.charAt(0).toUpperCase() + m.ping_prichina.slice(1) : "";
    const latency = typeof m?.ping_ms === "number" ? `${m.ping_ms} мс` : prichina || (m?.ping_otkaz ? "Недоступен" : "Не измерен");
    return <li key={key} className={`group flex min-h-12 items-center rounded-lg ${active || selected ? "bg-accent-soft" : "hover:bg-surface-hover"}`}>
      <button type="button" disabled={disabled || !naVybor} onClick={() => g.podpiska ? naVybor?.(s.id, g.id) : naVybor?.(s.id)}
        aria-label={`Подключиться к ${s.imya}`} aria-pressed={active || selected}
        className="flex min-w-0 flex-1 items-center gap-3 px-3 py-1 text-left disabled:opacity-45">
        <IkServer className={`h-5 w-5 shrink-0 ${active ? "text-accent-ink" : "text-fg-muted"}`} />
        <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium" title={s.imya}>{s.imya}</span>
          <span className="text-fg-muted block truncate text-xs" title={`${g.imya} · ${s.host}:${s.port}`}>{s.transport}{zakreplen ? ` · ${g.imya}` : ""}</span></span>
        <span className="text-fg-secondary shrink-0 text-xs tabular-nums" title={m?.ping_otkaz || undefined}>{latency}</span>
        {/* Отметка стоит только у выбранного. Пустой кружок в каждой строке
            ничего не сообщал: выбран ровно один, и это видно по нему одному.
            Место под отметку держится всегда, иначе колонка замера прыгает. */}
        <span className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full ${active || selected ? "bg-accent text-foreground" : ""}`}>
          {(active || selected) && <IkGalka className="h-3 w-3" />}
        </span>
      </button>
      <button type="button" aria-label={`${zakreplen ? "Открепить" : "Закрепить"} ${s.imya}`} aria-pressed={zakreplen}
        onClick={() => pomenyat("zakrepleny", key)} className={`mr-1 rounded-md p-2 hover:bg-fill ${zakreplen ? "text-accent-ink" : "text-fg-muted"}`}>
        <IkZakrepit className="h-4 w-4" />
      </button>
    </li>;
  };
  let naydeno = zakreplennye.length;
  const sektsii = gruppy.filter(g => !vid.skryty.includes(g.id)).map(g => {
    const rows = strokiGrupp.filter(r => r.g.id === g.id && !vid.zakrepleny.includes(r.key) && sovpadaet(r));
    naydeno += rows.length;
    if (z && !rows.length) return null;
    const zhdyot = obnovlyaetsya || obnovlyaemyePodpiski.includes(g.id);
    const svernuta = vid.svernuty.includes(g.id) && !z;
    return <section key={g.id} aria-label={g.imya} className="border-border border-t pt-1">
      <div className="flex min-h-10 items-center gap-1">
        <button type="button" aria-label={`${g.imya} · ${g.servery.length} ${slovoPosleChisla(g.servery.length, "сервер", "сервера", "серверов")}`} aria-expanded={!svernuta} onClick={() => pomenyat("svernuty", g.id)} className="flex min-w-0 flex-1 items-center gap-2 rounded px-2 py-2 text-left text-sm hover:bg-fill-subtle">
          <IkStrelkaVniz className={`h-4 w-4 shrink-0 ${svernuta ? "-rotate-90" : ""}`} />
          <span className="truncate font-medium" title={g.imya}>{g.imya}</span><span className="text-fg-muted text-xs">{g.servery.length}</span>
        </button>
        {g.podpiska && <>
          <button type="button" aria-busy={zhdyot} disabled={disabled || zhdyot || !naObnovit} aria-label={`Обновить ${g.imya}`} title="Обновить подписку" onClick={() => naObnovit?.(g.id)} className="text-fg-muted rounded p-2 hover:bg-fill disabled:opacity-40"><IkObnovit className={`h-4 w-4 ${zhdyot ? "animate-spin" : ""}`} /></button>
          <button type="button" onClick={() => pomenyat("skryty", g.id)} title="Скрыть из списка. VPN продолжит работать, закреплённые останутся видны." className="text-fg-muted rounded px-2 py-2 text-xs hover:bg-fill" aria-label={`Скрыть подписку ${g.imya}`}>Скрыть</button>
        </>}
      </div>
      {!svernuta && <>
        {g.podpiska?.otkaz && <p role="status" className="text-warn px-3 pb-2 text-xs">Не обновлена: {g.podpiska.otkaz}</p>}
        {g.podpiska?.obnovlena && <p className="text-fg-muted px-3 pb-1 text-xs">Обновлена {vozrast(g.podpiska.obnovlena)}</p>}
        <ul className="flex flex-col gap-0.5">{rows.map(r => ryad(r))}</ul>
        {!rows.length && <p className="text-fg-muted px-3 py-2 text-xs">{g.servery.length ? "Все серверы закреплены выше" : "Серверов пока нет. Обнови подписку."}</p>}
      </>}
    </section>;
  });
  return <div aria-label="Список серверов" className="flex flex-col gap-2">
    {state.otkaz && <p role="status" className="text-warn text-xs">{state.otkaz}</p>}
    {!!zakreplennye.length && <section aria-label="Закреплённые"><h3 className="text-fg-secondary flex items-center gap-2 px-2 pb-2 text-xs font-medium"><IkZakrepit className="h-4 w-4" />Закреплённые · {zakreplennye.length}</h3><ul>{zakreplennye.map(r => ryad(r, true))}</ul></section>}
    {sektsii}
    {!!z && !naydeno && <p className="text-fg-muted py-4 text-center text-sm">Ничего не найдено</p>}
    {!!skrytye.length && <div className="border-border border-t pt-2">
      <button type="button" aria-expanded={pokazatSkrytye} onClick={() => setPokazatSkrytye(!pokazatSkrytye)} className="text-accent-ink px-2 py-1 text-xs">Скрытые подписки · {skrytye.length}</button>
      {pokazatSkrytye && <><p className="text-fg-muted px-2 py-1 text-xs">Скрытие меняет только список. Закреплённые и VPN остаются доступны.</p>{skrytye.map(g => <div key={g.id} className="flex items-center justify-between gap-2 px-2 py-2 text-sm"><span className="truncate">{g.imya}</span><button type="button" className="text-accent-ink shrink-0" onClick={() => pomenyat("skryty", g.id)}>Показать {g.imya}</button></div>)}</>}
    </div>}
  </div>;
}
