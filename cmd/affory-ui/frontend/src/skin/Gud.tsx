// Пилюля AURUM: то, что на референсе заменяет собой всё окно. Имя сервера
// с флагом и таймером, переключатель режима, две круглые кнопки и «Настроить».
// Управляет настоящим экраном Affory через dom.ts, своего состояния о VPN не
// держит: что показано, то и есть на самом деле.
import { useEffect, useRef, useState } from "react";
import { Flag } from "./flagi";
import { naPodklyuchenie, minutyIzVSeti, pereklyuchit, useAffory, vyborMarshruta, type Snimok } from "./dom";
import { Derevo } from "./Derevo";
import { Panel } from "./Panel";
import { IkDerevo, IkKarta, IkTochki, IkVniz } from "./znachki";
import { useUstanovki, zadat } from "./ustanovki";

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

const SLOVO: Record<string, string> = {
  podnyat: "Защищено",
  podnimaetsya: "Подключаюсь…",
  vosstanavlivaetsya: "Восстанавливаю…",
  "sluzhba-molchit": "Служба не отвечает",
};

export function Gud() {
  const a = useAffory();
  const u = useUstanovki();
  const taymer = useTaymer(a);
  const [panel, setPanel] = useState(false);
  const [derevo, setDerevo] = useState(false);
  const [vybor, setVybor] = useState(false);
  const proshlyyFon = useRef(u.fon);
  const podnyat = a.sostoyanie === "podnyat";
  const zanyat = a.sostoyanie === "podnimaetsya" || a.sostoyanie === "vosstanavlivaetsya";

  // Пилюле нужен экран «Подключение»: если человек был на другой вкладке,
  // возвращаем его туда один раз при входе в режим.
  useEffect(() => { if (u.gud && !a.est) naPodklyuchenie(); }, [u.gud, a.est]);

  const vse = a.gruppy.flatMap((g) => g.servery);
  const imya = a.imya || (a.est ? "Сервер не выбран" : "Affory");

  const rezhim = async (m: "vpn" | "direct" | "off") => {
    if (m === "off") { if (podnyat || zanyat) pereklyuchit(); return; }
    vyborMarshruta(m);
    if (!podnyat && !zanyat) setTimeout(pereklyuchit, 80);
  };
  const aktiv: "off" | "vpn" | "direct" = !podnyat && !zanyat ? "off" : (a.marshrut ?? "vpn");

  return (
    <div className="aurum-gud" data-state={a.sostoyanie || "net"}>
      <div className="aurum-gud-kolonka">
        <button type="button" className="aurum-chip" onClick={() => setPanel((v) => !v)} aria-expanded={panel} data-testid="aurum-nastroit">
          Настроить
        </button>

        <div className="aurum-pilyulya-obertka">
          <button
            type="button"
            className="aurum-pilyulya"
            onClick={() => setVybor((v) => !v)}
            aria-haspopup="listbox"
            aria-expanded={vybor}
            disabled={!vse.length}
            data-testid="aurum-server"
          >
            <Flag imya={imya} razmer={34} />
            <span className="aurum-pilyulya-tekst">
              <b>{imya}</b>
              <small>{podnyat ? taymer : (SLOVO[a.sostoyanie] ?? (a.est ? "Отключено" : "Ожидание"))}</small>
            </span>
            {a.transport && <em className="aurum-protokol">{a.transport}</em>}
            <IkVniz className="aurum-strelka" />
          </button>
          {vybor && vse.length > 0 && (
            <ul className="aurum-vypad" role="listbox" aria-label="Сервер">
              {vse.map((s) => (
                <li key={s.imya + s.transport}>
                  <button type="button" role="option" aria-selected={s.aktiven} className={s.aktiven ? "is-on" : ""} onClick={() => { s.nazhat(); setVybor(false); }}>
                    <span className="aurum-galka">{s.aktiven ? "✓" : ""}</span>
                    <Flag imya={s.imya} razmer={20} />
                    <span className="aurum-vypad-imya">{s.imya}</span>
                    <small>{s.transport}</small>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="aurum-seg" role="radiogroup" aria-label="Режим" data-aktiv={aktiv}>
          <span className="aurum-seg-thumb" aria-hidden />
          {([["off", "Off"], ["direct", "Выбор"], ["vpn", "Всё"]] as const).map(([z, p]) => (
            <button key={z} type="button" role="radio" aria-checked={aktiv === z} disabled={!a.est || (a.zanyato && z !== "off")} onClick={() => void rezhim(z)} data-testid={`aurum-rezhim-${z}`}>
              {p}
            </button>
          ))}
        </div>

        <div className="aurum-krugi">
          <button type="button" className="aurum-krug" onClick={() => setDerevo((v) => !v)} aria-label="Серверы" aria-expanded={derevo} title="Серверы" data-testid="aurum-derevo">
            <IkDerevo />
          </button>
          <button
            type="button"
            className="aurum-krug"
            aria-label="Карта"
            title="Планета на фоне"
            aria-pressed={u.fon === "planeta"}
            onClick={() => { if (u.fon === "planeta") zadat({ fon: proshlyyFon.current === "planeta" ? "galaktika" : proshlyyFon.current }); else { proshlyyFon.current = u.fon; zadat({ fon: "planeta" }); } }}
          >
            <IkKarta />
          </button>
        </div>

        <div className="aurum-metriki" aria-label="Скорость">
          <span><i>↓</i>{podnyat ? a.priem || "—" : "—"}</span>
          <span><i>↑</i>{podnyat ? a.otdacha || "—" : "—"}</span>
          <span><i>◷</i>{podnyat ? a.zaderzhka || "—" : "—"}</span>
        </div>
        {!a.est && (
          <p className="aurum-podskazka">
            Здесь пока нет экрана подключения. Открой полное окно: <button type="button" onClick={() => zadat({ gud: false })}>развернуть</button>
          </p>
        )}
      </div>

      {derevo && <Derevo gruppy={a.gruppy} zakryt={() => setDerevo(false)} versiya={a.versiya} />}
      {panel && <Panel zakryt={() => setPanel(false)} />}
      <button type="button" className="aurum-tochki" onClick={() => zadat({ gud: false })} aria-label="Полное окно" title="Полное окно">
        <IkTochki />
      </button>
    </div>
  );
}
