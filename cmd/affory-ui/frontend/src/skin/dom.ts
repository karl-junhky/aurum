// Мост AURUM к экранам Affory. Оболочка НЕ импортирует ни одного модуля
// апстрима: она читает то, что экраны уже нарисовали (data-testid, aria-label,
// role), и нажимает те же кнопки, что нажал бы человек. Поэтому новые версии
// Affory подтягиваются без правок здесь: пока у экрана живы его якоря (на них
// держатся его тесты, а мост проверяет most-s-affory.test.ts), логика берётся
// оттуда, а вид наш.
import { useEffect, useState } from "react";

export type Sostoyanie = "podnyat" | "podnimaetsya" | "vosstanavlivaetsya" | "sluzhba-molchit" | string;

/** Один сервер из списка Affory: у него одно имя и один протокол. */
export interface Stroka {
  imya: string;
  /** Имя без протокола: ключ локации. */
  bazaImya: string;
  transport: string;
  ping: string;
  aktiven: boolean;
  nazhat: () => void;
}

/** Локация: один город или узел, у которого может быть несколько протоколов. */
export interface UzelLokatsii {
  imya: string;
  aktiven: boolean;
  ping: string;
  tekushchiyTransport: string;
  varianty: Stroka[];
}

export interface Gruppa {
  imya: string;
  servery: Stroka[];
  lokatsii: UzelLokatsii[];
}

export interface Snimok {
  /** Экран подключения смонтирован: без него пилюле управлять нечем. */
  est: boolean;
  sostoyanie: Sostoyanie;
  imya: string;
  transport: string;
  vSeti: string;
  priem: string;
  otdacha: string;
  zaderzhka: string;
  marshrut: "vpn" | "direct" | null;
  zanyato: boolean;
  gruppy: Gruppa[];
  versiya: string;
  /** Идёт замер отклика серверов (кнопка «Проверяю отклик»). */
  proverka: boolean;
  /** Кнопку замера можно нажать прямо сейчас. */
  proverkaMozhno: boolean;
}

const q = <T extends Element = HTMLElement>(s: string) => document.querySelector<T>(s);
const tekst = (s: string) => q(s)?.textContent?.trim() ?? "";

const SEKTSIYA = 'section[aria-label="Подключение"]';

function knopkaSekcii(podstroka: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll<HTMLButtonElement>(`${SEKTSIYA} button`)).find((b) => b.textContent?.includes(podstroka));
}

/** Нажимает «Проверить серверы»: замер делает сам Affory, мы только просим. */
export function zapustitProverku(): boolean {
  const b = knopkaSekcii("Проверить серверы");
  if (!b || b.disabled) return false;
  b.click();
  return true;
}

/** Просит Affory обновить подписки (кнопки «Обновить …» у групп) и замерить отклик. */
export function obnovitVsePodpiski(): void {
  document.querySelectorAll<HTMLButtonElement>('[aria-label="Список серверов"] button[aria-label^="Обновить "]').forEach((b) => { if (!b.disabled) b.click(); });
  zapustitProverku();
}

/** «Добавить сервер» живёт на главном экране полного окна. */
export function otkrytUpravlenieServerami(): void {
  naPodklyuchenie();
  setTimeout(() => knopkaSekcii("Добавить")?.click(), 40);
}

function sobratLokatsii(servery: Stroka[]): UzelLokatsii[] {
  const karta = new Map<string, Stroka[]>();
  for (const s of servery) karta.set(s.bazaImya, [...(karta.get(s.bazaImya) ?? []), s]);
  return [...karta].map(([imya, varianty]) => {
    const aktivnyy = varianty.find((v) => v.aktiven);
    const lyuboy = aktivnyy ?? varianty[0];
    return { imya, aktiven: !!aktivnyy, ping: lyuboy.ping, tekushchiyTransport: lyuboy.transport, varianty };
  });
}

export function snimok(): Snimok {
  const sek = q(SEKTSIYA);
  const imya = q('[data-testid="nesushchiy"] .affory-server-name')?.textContent?.trim() ?? tekst('[data-testid="nesushchiy"]');
  let marshrut: Snimok["marshrut"] = null;
  document.querySelectorAll<HTMLElement>('[role="radiogroup"][aria-label="Куда идёт трафик"] [role="radio"]').forEach((b, i) => {
    if (b.getAttribute("aria-checked") === "true") marshrut = i === 0 ? "vpn" : "direct";
  });
  const gruppy: Gruppa[] = [];
  document.querySelectorAll<HTMLElement>('[aria-label="Список серверов"] section[aria-label]').forEach((sec) => {
    const servery: Stroka[] = [];
    sec.querySelectorAll<HTMLElement>("li").forEach((li) => {
      const kn = li.querySelector<HTMLButtonElement>('button[aria-label^="Подключиться к "]');
      if (!kn) return;
      const sp = kn.querySelectorAll<HTMLElement>("span.truncate, span.tabular-nums");
      const polnoe = (kn.getAttribute("aria-label") ?? "").replace("Подключиться к ", "");
      servery.push({
        imya: polnoe,
        bazaImya: polnoe,
        transport: sp[1]?.textContent?.trim() ?? "",
        ping: kn.querySelector<HTMLElement>("span.tabular-nums")?.textContent?.trim() ?? "",
        aktiven: kn.getAttribute("aria-pressed") === "true",
        nazhat: () => kn.click(),
      });
    });
    if (servery.length) gruppy.push({ imya: sec.getAttribute("aria-label") ?? "", servery, lokatsii: sobratLokatsii(servery) });
  });
  return {
    est: !!sek,
    sostoyanie: sek?.getAttribute("data-state") ?? "",
    imya,
    transport: tekst(".affory-protocol"),
    vSeti: tekst('[data-testid="v-seti"]'),
    priem: tekst('[data-testid="skorost-priema"]'),
    otdacha: tekst('[data-testid="skorost-otdachi"]'),
    zaderzhka: tekst('[data-testid="zaderzhka"]'),
    marshrut,
    zanyato: !!q<HTMLButtonElement>('[data-testid="glavnoe-deystvie"]')?.disabled,
    gruppy,
    proverka: knopkaSekcii("Проверяю отклик") !== undefined,
    proverkaMozhno: knopkaSekcii("Проверить серверы")?.disabled === false,
    versiya: Array.from(document.querySelectorAll("footer b")).pop()?.textContent?.trim() ?? "",
  };
}

const klyuchi = (s: Snimok) =>
  JSON.stringify({ ...s, gruppy: s.gruppy.map((g) => [g.imya, g.servery.map((x) => [x.imya, x.transport, x.ping, x.aktiven])]) });

/** Живой снимок экрана Affory: наблюдатель за DOM плюс секундный тик. */
export function useAffory(): Snimok {
  const [s, setS] = useState<Snimok>(snimok);
  useEffect(() => {
    let tm = 0;
    const obnovit = () => { tm = 0; setS((p) => { const n = snimok(); return klyuchi(p) === klyuchi(n) ? p : n; }); };
    const planirovat = () => { if (!tm) tm = window.setTimeout(obnovit, 120); };
    const kor = document.getElementById("root");
    const mo = new MutationObserver(planirovat);
    if (kor) mo.observe(kor, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ["data-state", "aria-pressed", "aria-checked", "disabled"] });
    const iv = window.setInterval(obnovit, 1000);
    obnovit();
    return () => { mo.disconnect(); clearInterval(iv); clearTimeout(tm); };
  }, []);
  return s;
}

// ---- действия: те же кнопки, что нажимает человек ----

function vkladka(imya: string): HTMLElement | undefined {
  return Array.from(document.querySelectorAll<HTMLElement>('[role="tab"]')).find((b) => b.textContent?.includes(imya));
}
export const naPodklyuchenie = () => vkladka("Подключение")?.click();
export const naPravila = () => vkladka("Правила")?.click();
export const naSvoyVpn = () => vkladka("Свой VPN")?.click();
export const naNastroyki = () => vkladka("Настройки")?.click();

export const pereklyuchit = () => q<HTMLButtonElement>('[data-testid="glavnoe-deystvie"]')?.click();

export function vyborMarshruta(m: "vpn" | "direct"): boolean {
  const b = document.querySelectorAll<HTMLElement>('[role="radiogroup"][aria-label="Куда идёт трафик"] [role="radio"]')[m === "vpn" ? 0 : 1];
  if (!b || (b as HTMLButtonElement).disabled) return false;
  b.click();
  return true;
}

/** Минуты из «1 ч 05 мин» / «12 мин»: самое точное, что есть в DOM. */
export function minutyIzVSeti(s: string): number | null {
  const m = /^(?:(\d+)\s*ч\s*)?(\d+)\s*мин/.exec(s);
  return m ? (m[1] ? +m[1] * 60 : 0) + +m[2] : null;
}
