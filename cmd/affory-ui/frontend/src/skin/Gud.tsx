// Пилюля AURUM: то, что на референсе заменяет собой всё окно. Имя сервера с
// флагом и таймером, переключатель Off / Proxy / TUN, две круглые кнопки,
// «Настроить» и боковая панель серверов. Управляет настоящим экраном Affory
// через dom.ts, своего состояния о VPN не держит: что показано, то и есть.
import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { Flag } from "./flagi";
import { luchshayaStrana, sobratStrany, stoitLiMenyat } from "./analiz";
import { minutyIzVSeti, naPodklyuchenie, naPravila, pereklyuchit, vyborMarshruta, type Snimok, type Stroka } from "./dom";
import { Servery, podklyuchitStranu } from "./Servery";
import { Panel } from "./Panel";
import { opublikovat, povernutK, useZhivoe } from "./store";
import { useUstanovki, zadat } from "./ustanovki";
import { IkDerevo, IkKarta, IkMolniya, IkShesterenka, IkTochki, IkVniz } from "./znachki";

const dvoe = (n: number) => String(n).padStart(2, "0");
export function formatVremeni(sek: number): string {
  const s = Math.max(0, Math.floor(sek));
  return `${dvoe(Math.floor(s / 3600))}:${dvoe(Math.floor((s % 3600) / 60))}:${dvoe(s % 60)}`;
}

/** Секундомер «в сети»: DOM знает только минуты, секунды досчитываем сами и
 *  каждый раз сверяемся, чтобы не уплыть. */
function useTaymer(a: Snimok): string {
  const nach = useRef<number | null>(null);
  const [, tik] = useState(0);
  useEffect(() => { const i = setInterval(() => tik((n) => n + 1), 500); return () => clearInterval(i); }, []);
  if (a.sostoyanie !== "podnyat") { nach.current = null; return "00:00:00"; }
  const min = minutyIzVSeti(a.vSeti);
  const seychas = Date.now();
  if (nach.current === null) nach.current = seychas - (min ?? 0) * 60000;
  else if (min !== null && Math.floor((seychas - nach.current) / 60000) !== min) nach.current = seychas - min * 60000 - 1000;
  return formatVremeni((seychas - nach.current) / 1000);
}

/** Закрывает всплывающее по нажатию снаружи и по Escape. */
function useZakrytie(ref: RefObject<HTMLElement | null>, otkryto: boolean, zakryt: () => void) {
  useEffect(() => {
    if (!otkryto) return;
    const vne = (e: PointerEvent) => { if (!ref.current?.contains(e.target as Node)) zakryt(); };
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") zakryt(); };
    window.addEventListener("pointerdown", vne);
    window.addEventListener("keydown", esc);
    return () => { window.removeEventListener("pointerdown", vne); window.removeEventListener("keydown", esc); };
  }, [ref, otkryto, zakryt]);
}

const SLOVO: Record<string, string> = {
  podnimaetsya: "Подключаюсь…",
  vosstanavlivaetsya: "Восстанавливаю…",
  "sluzhba-molchit": "Служба не отвечает",
};

export function Gud({ a }: { a: Snimok }) {
  const u = useUstanovki();
  const zh = useZhivoe();
  const taymer = useTaymer(a);
  const [panel, setPanel] = useState(false);
  const [derevo, setDerevo] = useState(() => window.innerWidth >= 1000);
  const [vybor, setVybor] = useState(false);
  const [karta, setKarta] = useState(false);
  const obertkaVybora = useRef<HTMLDivElement>(null);
  const obertkaKarty = useRef<HTMLDivElement>(null);
  useZakrytie(obertkaVybora, vybor, () => setVybor(false));
  useZakrytie(obertkaKarty, karta, () => setKarta(false));

  const podnyat = a.sostoyanie === "podnyat";
  const zanyat = a.sostoyanie === "podnimaetsya" || a.sostoyanie === "vosstanavlivaetsya";
  useEffect(() => { if (!a.est) naPodklyuchenie(); }, [a.est]);

  // сообщение автовыбора живёт недолго
  useEffect(() => {
    if (!zh.izvestie) return;
    const id = setTimeout(() => opublikovat({ izvestie: "" }), 6500);
    return () => clearTimeout(id);
  }, [zh.izvestie]);

  const imya = a.imya || (a.est ? "Сервер не выбран" : "AURUM");
  const strany = useMemo(() => sobratStrany(a.gruppy), [a.gruppy]);
  const luchshaya = luchshayaStrana(strany);
  const tekushchaya = strany.find((s) => s.aktivna) ?? null;
  const predlozhit = !!luchshaya && !u.avtoVybor && (podnyat ? stoitLiMenyat(tekushchaya, luchshaya) : !!luchshaya && !luchshaya.aktivna && a.est);

  // протоколы текущей локации, а если он один, то все серверы
  const lokatsiya = a.gruppy.flatMap((g) => g.lokatsii).find((l) => l.aktiven);
  const protokoly = lokatsiya && lokatsiya.varianty.length > 1;
  const punkty: Stroka[] = protokoly ? lokatsiya.varianty : a.gruppy.flatMap((g) => g.servery).slice(0, 12);

  const rezhim = (m: "vpn" | "direct" | "off") => {
    if (m === "off") { if (podnyat || zanyat) pereklyuchit(); return; }
    vyborMarshruta(m);
    if (!podnyat && !zanyat) setTimeout(pereklyuchit, 80);
  };
  const aktiv: "off" | "direct" | "vpn" = !podnyat && !zanyat ? "off" : (a.marshrut ?? "vpn");
  const planeta = u.fon === "planeta";
  const proshlyyFon = useRef(u.fon);

  return (
    <div className="aurum-gud" data-state={a.sostoyanie || "net"} data-derevo={derevo ? "1" : "0"}>
      <button type="button" className="aurum-titlebar-gear" onClick={() => setPanel((v) => !v)} aria-expanded={panel} aria-label="Настроить" title="Оформление и автопроверка" data-testid="aurum-nastroit">
        <IkShesterenka />
      </button>
      <button type="button" className="aurum-tochki" onClick={() => zadat({ gud: false })} aria-label="Полное окно" title="Полное окно Affory">
        <IkTochki />
      </button>

      {derevo && <Servery a={a} zakryt={() => setDerevo(false)} />}

      <div className="aurum-gud-kolonka">
        <div className="aurum-hover-chip-wrap">
          <button type="button" className="aurum-chip" onClick={() => setPanel(true)}>Настроить</button>
        </div>

        <div className="aurum-pilyulya-obertka" ref={obertkaVybora}>
          <button type="button" className="aurum-pilyulya" onClick={() => { setKarta(false); setVybor((v) => !v); }} aria-haspopup="listbox" aria-expanded={vybor} disabled={!punkty.length} data-testid="aurum-server">
            <Flag imya={imya} razmer={34} />
            <span className="aurum-pilyulya-tekst">
              <b>{imya}</b>
              <small>{podnyat ? taymer : (SLOVO[a.sostoyanie] ?? (a.est ? "00:00:00" : "Ожидание"))}</small>
            </span>
            {a.transport && <em className="aurum-protokol">{a.transport}</em>}
            <IkVniz className="aurum-strelka" />
          </button>
          {vybor && punkty.length > 0 && (
            <ul className="aurum-popover aurum-popover-protokoly" role="listbox" aria-label={protokoly ? "Протокол" : "Сервер"}>
              <li className="aurum-popover-zagolovok">{protokoly ? "Протокол" : "Сервер"}</li>
              {punkty.map((s) => (
                <li key={s.imya + s.transport}>
                  <button type="button" role="option" aria-selected={s.aktiven} className={s.aktiven ? "is-on" : ""} onClick={() => { s.nazhat(); setVybor(false); }}>
                    <span className="aurum-galka">{s.aktiven ? "✓" : ""}</span>
                    {!protokoly && <Flag imya={s.imya} razmer={20} />}
                    <span className="aurum-vypad-imya">{protokoly ? s.transport : s.imya}</span>
                    {!protokoly && <small>{s.transport}</small>}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="aurum-seg" role="radiogroup" aria-label="Режим" data-aktiv={aktiv}>
          <span className="aurum-seg-thumb" aria-hidden />
          {([["off", "Off", "Выключить VPN"], ["direct", "Proxy", "Только выбранное: правила решают, что идёт через VPN"], ["vpn", "TUN", "Весь трафик через VPN"]] as const).map(([z, p, podskazka]) => (
            <button key={z} type="button" role="radio" aria-checked={aktiv === z} title={podskazka} disabled={!a.est || (a.zanyato && z !== "off")} onClick={() => rezhim(z)} data-testid={`aurum-rezhim-${z}`}>
              {p}
            </button>
          ))}
        </div>

        <div className="aurum-krugi">
          <button type="button" className="aurum-krug" onClick={() => setDerevo((v) => !v)} aria-label="Серверы" aria-expanded={derevo} title="Список серверов и анализ" data-testid="aurum-derevo">
            <IkDerevo />
          </button>
          <div className="aurum-karta-obertka" ref={obertkaKarty}>
            <button type="button" className="aurum-krug" aria-label="Маршрутизация и карта" aria-expanded={karta} onClick={() => { setVybor(false); setKarta((v) => !v); }}>
              <IkKarta />
            </button>
            {karta && (
              <ul className="aurum-popover aurum-popover-routing" role="menu">
                <li><button type="button" className={aktiv === "vpn" ? "is-on" : ""} onClick={() => { rezhim("vpn"); setKarta(false); }}><span className="aurum-galka">{aktiv === "vpn" ? "✓" : ""}</span><span>Весь трафик (TUN)</span></button></li>
                <li><button type="button" className={aktiv === "direct" ? "is-on" : ""} onClick={() => { rezhim("direct"); setKarta(false); }}><span className="aurum-galka">{aktiv === "direct" ? "✓" : ""}</span><span>Только выбранное (Proxy)</span></button></li>
                <li className="aurum-popover-sep" />
                <li><button type="button" onClick={() => { if (planeta) zadat({ fon: proshlyyFon.current === "planeta" ? "perelivy" : proshlyyFon.current }); else { proshlyyFon.current = u.fon; zadat({ fon: "planeta" }); } setKarta(false); }}><span className="aurum-galka">{planeta ? "✓" : ""}</span><span>Планета на фоне</span></button></li>
                <li className="aurum-popover-sep" />
                <li><button type="button" onClick={() => { setKarta(false); zadat({ gud: false }); setTimeout(naPravila, 60); }}><span className="aurum-galka" /><span>Правила обхода…</span></button></li>
              </ul>
            )}
          </div>
        </div>

        {predlozhit && luchshaya && luchshaya.ping !== null && (
          <button type="button" className="aurum-sovet" onClick={() => { podklyuchitStranu(luchshaya); povernutK(luchshaya.kod); }}>
            <IkMolniya /> {podnyat ? "Быстрее" : "Лучшая сейчас"}: {luchshaya.imya} · {luchshaya.ping} мс
          </button>
        )}
        {u.pokazatSkorost && (
          <div className="aurum-metriki" aria-label="Скорость">
            <span><i>↓</i>{podnyat ? a.priem || "—" : "—"}</span>
            <span><i>↑</i>{podnyat ? a.otdacha || "—" : "—"}</span>
            <span><i>◷</i>{podnyat ? a.zaderzhka || "—" : "—"}</span>
          </div>
        )}
        {!a.est && <p className="aurum-podskazka">Экран подключения не открыт. <button type="button" onClick={() => zadat({ gud: false })}>Развернуть окно</button></p>}
      </div>

      {zh.izvestie && <div className="aurum-toast" role="status">{zh.izvestie}</div>}
      {panel && <Panel zakryt={() => setPanel(false)} />}
    </div>
  );
}
