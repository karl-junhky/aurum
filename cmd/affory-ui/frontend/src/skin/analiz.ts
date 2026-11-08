// Анализ лучшей страны. Чистые функции над тем, что Affory уже намерил:
// пинг каждого сервера берётся из его строки. Сам замер делает Affory.
import { STRANA, stranaPoImeni } from "./geo";
import type { Gruppa, Stroka } from "./dom";

export type Ton = "ok" | "mid" | "bad" | "";

export interface StranaSvodka {
  klyuch: string;
  kod: string | null;
  imya: string;
  servery: Stroka[];
  /** Самый быстрый из отвечающих серверов страны. */
  luchshiy: Stroka | null;
  ping: number | null;
  srednee: number | null;
  /** Меньше значит лучше. Лучший пинг плюс четверть разброса: страна, где
   *  отвечает один сервер из пяти, не обгоняет страну с ровными двумя. */
  ochki: number | null;
  ton: Ton;
  aktivna: boolean;
  neotvechayut: number;
  izmereno: number;
}

/** «45 мс» -> 45. «Не измерен» и «Недоступен» дают null. */
export function pingChislo(tekst: string): number | null {
  const m = /^(\d+(?:[.,]\d+)?)\s*мс/.exec(tekst.trim());
  return m ? Math.round(parseFloat(m[1].replace(",", "."))) : null;
}

export function tonPinga(ms: number | null): Ton {
  if (ms === null) return "";
  return ms < 60 ? "ok" : ms < 150 ? "mid" : "bad";
}

export function slovoPinga(ms: number | null): string {
  if (ms === null) return "нет замера";
  return ms < 40 ? "отлично" : ms < 80 ? "очень хорошо" : ms < 150 ? "нормально" : "медленно";
}

const NE_ZAMERYAN = /не измерен/i;

export function sobratStrany(gruppy: Gruppa[]): StranaSvodka[] {
  const karta = new Map<string, StranaSvodka>();
  for (const g of gruppy) {
    for (const s of g.servery) {
      const kod = stranaPoImeni(s.imya);
      const klyuch = kod ?? `imya:${s.imya.split("·")[0].trim().toLowerCase()}`;
      let st = karta.get(klyuch);
      if (!st) {
        st = {
          klyuch, kod, imya: kod ? STRANA[kod].imya : s.imya.split("·")[0].trim(),
          servery: [], luchshiy: null, ping: null, srednee: null, ochki: null, ton: "", aktivna: false, neotvechayut: 0, izmereno: 0,
        };
        karta.set(klyuch, st);
      }
      st.servery.push(s);
      if (s.aktiven) st.aktivna = true;
    }
  }
  for (const st of karta.values()) {
    const pingi = st.servery.map((s) => ({ s, ms: pingChislo(s.ping) }));
    const ok = pingi.filter((p): p is { s: Stroka; ms: number } => p.ms !== null);
    st.izmereno = st.servery.filter((s) => !NE_ZAMERYAN.test(s.ping)).length;
    st.neotvechayut = st.izmereno - ok.length;
    if (ok.length) {
      ok.sort((a, b) => a.ms - b.ms);
      st.luchshiy = ok[0].s;
      st.ping = ok[0].ms;
      st.srednee = Math.round(ok.reduce((n, p) => n + p.ms, 0) / ok.length);
      st.ochki = st.ping + 0.25 * (st.srednee - st.ping) + st.neotvechayut * 8;
      st.ton = tonPinga(st.ping);
    }
  }
  return [...karta.values()].sort((a, b) => (a.ochki ?? Infinity) - (b.ochki ?? Infinity) || a.imya.localeCompare(b.imya, "ru"));
}

export function luchshayaStrana(strany: StranaSvodka[]): StranaSvodka | null {
  return strany.find((s) => s.ochki !== null) ?? null;
}

/** Стоит ли уходить с текущей страны: новая быстрее хотя бы на 20 мс и на
 *  треть, либо текущая вовсе не отвечает. Мелкая разница не повод рвать
 *  работающее соединение. */
export function stoitLiMenyat(tekushchaya: StranaSvodka | null, luchshaya: StranaSvodka | null): boolean {
  if (!luchshaya || luchshaya.ping === null || !luchshaya.luchshiy) return false;
  if (!tekushchaya || tekushchaya.klyuch === luchshaya.klyuch) return false;
  if (tekushchaya.ping === null) return tekushchaya.izmereno > 0;
  return tekushchaya.ping - luchshaya.ping >= 20 && luchshaya.ping <= tekushchaya.ping * 0.67;
}

/** Доля шкалы для полоски: 0..1, быстрее значит длиннее. */
export function dolyaShkaly(ms: number | null): number {
  if (ms === null) return 0.04;
  return Math.max(0.06, Math.min(1, 1 - Math.log(Math.max(ms, 8) / 8) / Math.log(400 / 8)));
}
