// Мост AURUM к экранам Affory. Оболочка НЕ импортирует ни одного модуля
// апстрима: она читает то, что экраны уже нарисовали (data-testid, aria-label,
// role), и нажимает те же кнопки, что нажал бы человек. Поэтому новые версии
// Affory подтягиваются без правок здесь: пока у экрана живы его testid'ы (на них
// держатся его собственные тесты), логика берётся оттуда, а вид наш.
import { useEffect, useState } from "react";

export type Sostoyanie = "podnyat" | "podnimaetsya" | "vosstanavlivaetsya" | "sluzhba-molchit" | string;

export interface Stroka {
  imya: string;
  transport: string;
  ping: string;
  aktiven: boolean;
  nazhat: () => void;
}
export interface Gruppa {
  imya: string;
  servery: Stroka[];
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
}

const q = <T extends Element = HTMLElement>(s: string) => document.querySelector<T>(s);
const tekst = (s: string) => q(s)?.textContent?.trim() ?? "";

export function snimok(): Snimok {
  const sek = q('section[aria-label="Подключение"]');
  const sost = sek?.getAttribute("data-state") ?? "";
  const imya = q('[data-testid="nesushchiy"] .affory-server-name')?.textContent?.trim() ?? tekst('[data-testid="nesushchiy"]');
  const marsh = document.querySelectorAll<HTMLElement>('[role="radiogroup"][aria-label="Куда идёт трафик"] [role="radio"]');
  let marshrut: Snimok["marshrut"] = null;
  marsh.forEach((b, i) => { if (b.getAttribute("aria-checked") === "true") marshrut = i === 0 ? "vpn" : "direct"; });
  const gruppy: Gruppa[] = [];
  document.querySelectorAll<HTMLElement>('[aria-label="Список серверов"] section[aria-label]').forEach((sec) => {
    const imyaG = sec.getAttribute("aria-label") ?? "";
    const servery: Stroka[] = [];
    sec.querySelectorAll<HTMLElement>("li").forEach((li) => {
      const kn = li.querySelector<HTMLButtonElement>('button[aria-label^="Подключиться к "]');
      if (!kn) return;
      const sp = kn.querySelectorAll<HTMLElement>("span.truncate, span.tabular-nums");
      servery.push({
        imya: (kn.getAttribute("aria-label") ?? "").replace("Подключиться к ", ""),
        transport: sp[1]?.textContent?.trim() ?? "",
        ping: kn.querySelector<HTMLElement>("span.tabular-nums")?.textContent?.trim() ?? "",
        aktiven: kn.getAttribute("aria-pressed") === "true",
        nazhat: () => kn.click(),
      });
    });
    if (servery.length) gruppy.push({ imya: imyaG, servery });
  });
  return {
    est: !!sek,
    sostoyanie: sost,
    imya,
    transport: tekst(".affory-protocol"),
    vSeti: tekst('[data-testid="v-seti"]'),
    priem: tekst('[data-testid="skorost-priema"]'),
    otdacha: tekst('[data-testid="skorost-otdachi"]'),
    zaderzhka: tekst('[data-testid="zaderzhka"]'),
    marshrut,
    zanyato: !!q<HTMLButtonElement>('[data-testid="glavnoe-deystvie"]')?.disabled,
    gruppy,
    versiya: Array.from(document.querySelectorAll("footer b")).pop()?.textContent?.trim() ?? "",
  };
}

function ravny(a: Snimok, b: Snimok): boolean {
  return JSON.stringify({ ...a, gruppy: a.gruppy.map((g) => g.servery.map((s) => [s.imya, s.ping, s.aktiven])) }) ===
    JSON.stringify({ ...b, gruppy: b.gruppy.map((g) => g.servery.map((s) => [s.imya, s.ping, s.aktiven])) });
}

/** Живой снимок экрана Affory: наблюдатель за DOM плюс секундный тик. */
export function useAffory(): Snimok {
  const [s, setS] = useState<Snimok>(snimok);
  useEffect(() => {
    let tm = 0;
    const obnovit = () => { tm = 0; setS((p) => { const n = snimok(); return ravny(p, n) ? p : n; }); };
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

/** Секунды в сети из «1 ч 05 мин» / «12 мин»: самое точное, что есть в DOM. */
export function minutyIzVSeti(s: string): number | null {
  const m = /^(?:(\d+)\s*ч\s*)?(\d+)\s*мин/.exec(s);
  return m ? (m[1] ? +m[1] * 60 : 0) + +m[2] : null;
}
