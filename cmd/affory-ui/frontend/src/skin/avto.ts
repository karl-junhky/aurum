// Автопроверка пинга и автовыбор лучшей страны. Работает в фоне, пока открыто
// окно: сам замер выполняет Affory (мы нажимаем его кнопку), мы лишь вовремя
// просим и читаем итог.
import { useEffect, useRef } from "react";
import { STRANA } from "./geo";
import { luchshayaStrana, sobratStrany, stoitLiMenyat } from "./analiz";
import { snimok, zapustitProverku, type Snimok } from "./dom";
import { SOBYTIE_METKI, opublikovat, povernutK, zhivoe } from "./store";
import { ustanovki } from "./ustanovki";

/** Публикует метки для планеты и следит за часами замера. */
export function useAvto(a: Snimok): void {
  const prezhnyaya = useRef(false);
  const posledniyZapusk = useRef(0);

  // метки планеты
  useEffect(() => {
    const strany = sobratStrany(a.gruppy);
    const metki = strany.flatMap((s) => (s.kod ? [{ klyuch: s.klyuch, kod: s.kod, imya: s.imya, lat: STRANA[s.kod].lat, lon: STRANA[s.kod].lon, ping: s.ping, ton: s.ton, aktivna: s.aktivna, servery: s.servery.length }] : []));
    opublikovat({ metki });
  }, [a.gruppy]);

  // конец замера: отмечаем время и, если включено, меняем страну
  useEffect(() => {
    if (prezhnyaya.current && !a.proverka) {
      opublikovat({ zameryAt: Date.now() });
      const u = ustanovki();
      if (u.avtoVybor && !a.zanyato && a.sostoyanie === "podnyat") {
        const strany = sobratStrany(a.gruppy);
        const luchshaya = luchshayaStrana(strany);
        const tekushchaya = strany.find((s) => s.aktivna) ?? null;
        if (stoitLiMenyat(tekushchaya, luchshaya) && luchshaya?.luchshiy) {
          luchshaya.luchshiy.nazhat();
          if (luchshaya.kod) povernutK(luchshaya.kod);
          opublikovat({ izvestie: `Переключил на ${luchshaya.imya}: ${luchshaya.ping} мс` });
        }
      }
    }
    prezhnyaya.current = a.proverka;
  }, [a.proverka, a.gruppy, a.zanyato, a.sostoyanie]);

  // нажали на метку на планете: подключаем лучший сервер этой страны
  useEffect(() => {
    const f = (e: Event) => {
      const klyuch = (e as CustomEvent<{ klyuch: string }>).detail?.klyuch;
      const st = sobratStrany(snimok().gruppy).find((x) => x.klyuch === klyuch);
      if (!st) return;
      (st.luchshiy ?? st.servery[0])?.nazhat();
      if (st.kod) povernutK(st.kod);
    };
    window.addEventListener(SOBYTIE_METKI, f);
    return () => window.removeEventListener(SOBYTIE_METKI, f);
  }, []);

  // таймер
  useEffect(() => {
    const tik = () => {
      const u = ustanovki();
      const s = snimok();
      if (!u.avtoMin || !s.est || !s.proverkaMozhno || s.proverka || s.zanyato) return;
      const proshlo = Date.now() - Math.max(zhivoe().zameryAt, posledniyZapusk.current);
      // первый замер через пару секунд после появления серверов, дальше по интервалу
      const nuzhno = zhivoe().zameryAt === 0 && posledniyZapusk.current === 0 ? s.gruppy.length > 0 : proshlo >= u.avtoMin * 60_000;
      if (nuzhno && zapustitProverku()) posledniyZapusk.current = Date.now();
    };
    const id = window.setInterval(tik, 4000);
    return () => clearInterval(id);
  }, []);
}
