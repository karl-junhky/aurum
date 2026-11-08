// @vitest-environment node
import { describe, expect, it } from "vitest";
import { dolyaShkaly, luchshayaStrana, pingChislo, sobratStrany, stoitLiMenyat } from "./analiz";
import { stranaPoImeni } from "./geo";
import type { Gruppa, Stroka } from "./dom";

const s = (imya: string, ping: string, aktiven = false): Stroka => ({ imya, bazaImya: imya, transport: "tcp", ping, aktiven, nazhat: () => {} });
const g = (...servery: Stroka[]): Gruppa[] => [{ imya: "г", servery, lokatsii: [] }];

describe("анализ лучшей страны", () => {
  it("читает пинг из строки сервера", () => {
    expect(pingChislo("45 мс")).toBe(45);
    expect(pingChislo("Не измерен")).toBeNull();
    expect(pingChislo("Недоступен")).toBeNull();
  });

  it("узнаёт страну по русскому и английскому имени", () => {
    expect(stranaPoImeni("Нидерланды · Амстердам")).toBe("NL");
    expect(stranaPoImeni("Estonia 1")).toBe("EE");
    expect(stranaPoImeni("Запасной сервер")).toBeNull();
  });

  it("лучшая страна та, где пинг ниже, а неизмеренные идут в конец", () => {
    const strany = sobratStrany(g(s("Германия · Франкфурт", "80 мс"), s("Финляндия · Хельсинки", "35 мс"), s("Польша · Варшава", "Не измерен")));
    expect(strany.map((x) => x.kod)).toEqual(["FI", "DE", "PL"]);
    expect(luchshayaStrana(strany)?.kod).toBe("FI");
  });

  it("серверы одной страны собираются в одну запись", () => {
    const strany = sobratStrany(g(s("Германия · Франкфурт 1", "60 мс"), s("Германия · Мюнхен", "40 мс")));
    expect(strany).toHaveLength(1);
    expect(strany[0].ping).toBe(40);
    expect(strany[0].luchshiy?.imya).toBe("Германия · Мюнхен");
  });

  it("не меняет страну из-за мелочи, но уходит с мёртвой", () => {
    const a = sobratStrany(g(s("Германия · А", "50 мс", true), s("Финляндия · Б", "40 мс")));
    expect(stoitLiMenyat(a.find((x) => x.aktivna) ?? null, luchshayaStrana(a))).toBe(false);
    const b = sobratStrany(g(s("Германия · А", "200 мс", true), s("Финляндия · Б", "40 мс")));
    expect(stoitLiMenyat(b.find((x) => x.aktivna) ?? null, luchshayaStrana(b))).toBe(true);
    const v = sobratStrany(g(s("Германия · А", "Недоступен", true), s("Финляндия · Б", "90 мс")));
    expect(stoitLiMenyat(v.find((x) => x.aktivna) ?? null, luchshayaStrana(v))).toBe(true);
  });

  it("полоска короче у медленных", () => {
    expect(dolyaShkaly(20)).toBeGreaterThan(dolyaShkaly(200));
    expect(dolyaShkaly(null)).toBeLessThan(0.1);
  });
});
