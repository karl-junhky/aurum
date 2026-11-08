// Боковая панель «Серверы»: анализ лучшей страны, замер по таймеру и список
// стран с локациями и протоколами. Читает то, что намерил Affory (dom.ts).
import { useMemo, useState } from "react";
import { Flag } from "./flagi";
import { dolyaShkaly, luchshayaStrana, slovoPinga, sobratStrany, type StranaSvodka } from "./analiz";
import { obnovitVsePodpiski, otkrytUpravlenieServerami, type Snimok, type Stroka } from "./dom";
import { povernutK, useZhivoe } from "./store";
import { useUstanovki, zadat } from "./ustanovki";
import { Tumbler } from "./elementy";
import { IkMolniya, IkObnovit, IkPlyus, IkPoisk, IkVniz, IkZakryt } from "./znachki";

function kogda(ms: number, seychas: number): string {
  if (!ms) return "ещё не проверяли";
  const m = Math.floor((seychas - ms) / 60000);
  return m < 1 ? "проверено только что" : `проверено ${m} мин назад`;
}

export function podklyuchitStranu(s: StranaSvodka): void {
  (s.luchshiy ?? s.servery[0])?.nazhat();
  if (s.kod) povernutK(s.kod);
}

function Protokoly({ servery }: { servery: Stroka[] }) {
  const lokatsii = new Map<string, Stroka[]>();
  for (const s of servery) lokatsii.set(s.bazaImya, [...(lokatsii.get(s.bazaImya) ?? []), s]);
  return (
    <ul className="aurum-lokatsii">
      {[...lokatsii].map(([imya, varianty]) => (
        <li key={imya}>
          <span className="aurum-lokatsiya-imya">{imya.includes("·") ? imya.split("·").slice(1).join("·").trim() : imya}</span>
          <span className="aurum-chipy">
            {varianty.map((v) => (
              <button key={v.transport} type="button" className={v.aktiven ? "is-on" : ""} aria-pressed={v.aktiven} onClick={v.nazhat}>
                {v.transport || "—"}{v.ping && v.ping !== "Не измерен" ? <small>{v.ping.replace(" мс", "")}</small> : null}
              </button>
            ))}
          </span>
        </li>
      ))}
    </ul>
  );
}

export function Servery({ a, zakryt }: { a: Snimok; zakryt: () => void }) {
  const u = useUstanovki();
  const zh = useZhivoe();
  const [zapros, setZapros] = useState("");
  const [raskryty, setRaskryty] = useState<string[]>([]);
  const strany = useMemo(() => sobratStrany(a.gruppy), [a.gruppy]);
  const luchshaya = luchshayaStrana(strany);
  const z = zapros.trim().toLowerCase();
  const spisok = strany.filter((s) => !z || `${s.imya} ${s.servery.map((x) => `${x.imya} ${x.transport}`).join(" ")}`.toLowerCase().includes(z));

  return (
    <aside className="aurum-derevo" aria-label="Серверы" data-testid="aurum-derevo-panel">
      <header className="aurum-derevo-shapka">
        <div>
          <b className="aurum-derevo-zagolovok">Серверы</b>
          <small>{a.proverka ? "идёт проверка…" : kogda(zh.zameryAt, Date.now())}{u.avtoMin ? ` · авто ${u.avtoMin} мин` : ""}</small>
        </div>
        <div className="aurum-derevo-knopki">
          <button type="button" className="aurum-ik-knopka" onClick={otkrytUpravlenieServerami} aria-label="Добавить сервер" title="Добавить сервер или подписку"><IkPlyus /></button>
          <button type="button" className={`aurum-ik-knopka ${a.proverka ? "is-spin" : ""}`} onClick={obnovitVsePodpiski} aria-label="Проверить серверы" title="Обновить подписки и измерить пинг"><IkObnovit /></button>
          <button type="button" className="aurum-ik-knopka" onClick={zakryt} aria-label="Закрыть список"><IkZakryt /></button>
        </div>
      </header>

      {luchshaya && luchshaya.ping !== null && (
        <section className="aurum-luchshaya" aria-label="Лучший выбор" data-ton={luchshaya.ton}>
          <span className="aurum-luchshaya-metka"><IkMolniya /> Лучший выбор</span>
          <div className="aurum-luchshaya-telo">
            <Flag imya={luchshaya.luchshiy?.imya ?? luchshaya.imya} razmer={40} />
            <div>
              <b>{luchshaya.imya}</b>
              <small>{luchshaya.ping} мс · {slovoPinga(luchshaya.ping)}{luchshaya.servery.length > 1 ? ` · ${luchshaya.servery.length} узла` : ""}</small>
            </div>
            <button type="button" className="aurum-knopka-glavnaya" disabled={luchshaya.aktivna} onClick={() => podklyuchitStranu(luchshaya)}>
              {luchshaya.aktivna ? "Выбрано" : "Выбрать"}
            </button>
          </div>
          <span className="aurum-poloska"><i style={{ transform: `scaleX(${dolyaShkaly(luchshaya.ping)})` }} /></span>
        </section>
      )}

      <label className="aurum-stroka-nastroyki">
        <span><b>Автовыбор лучшей страны</b><small>После проверки сам перейдёт, если быстрее на треть</small></span>
        <Tumbler on={u.avtoVybor} onChange={(v) => zadat({ avtoVybor: v })} label="Автовыбор лучшей страны" />
      </label>

      <label className="aurum-poisk">
        <IkPoisk />
        <input value={zapros} onChange={(e) => setZapros(e.target.value)} placeholder="Найти страну или протокол" aria-label="Найти сервер" />
      </label>

      <div className="aurum-derevo-spisok">
        {strany.length === 0 && (
          <div className="aurum-pusto-blok">
            <p className="aurum-pusto">Серверов пока нет.</p>
            <button type="button" className="aurum-knopka-glavnaya" onClick={otkrytUpravlenieServerami}><IkPlyus /> Добавить подписку</button>
          </div>
        )}
        {spisok.map((s) => {
          const otkryta = raskryty.includes(s.klyuch) || (!!z && s.servery.length > 0);
          return (
            <div key={s.klyuch} className={`aurum-strana ${s.aktivna ? "is-on" : ""}`} data-ton={s.ton}>
              <div className="aurum-strana-glavnaya">
                <button type="button" className="aurum-strana-knopka" aria-pressed={s.aktivna} onClick={() => podklyuchitStranu(s)}>
                  <Flag imya={s.luchshiy?.imya ?? s.imya} razmer={30} />
                  <span className="aurum-derevo-imya"><b>{s.imya}</b><small>{s.servery.length} {s.servery.length === 1 ? "узел" : "узла"}{s.neotvechayut ? ` · ${s.neotvechayut} не отвечает` : ""}</small></span>
                  <span className="aurum-ping" data-ton={s.ton}>{s.ping !== null ? `${s.ping} мс` : "—"}</span>
                </button>
                <button type="button" className="aurum-uzel-strelka" aria-expanded={otkryta} aria-label={`Протоколы ${s.imya}`} onClick={() => setRaskryty((p) => (p.includes(s.klyuch) ? p.filter((x) => x !== s.klyuch) : [...p, s.klyuch]))}>
                  <IkVniz className={otkryta ? "is-otkr" : ""} />
                </button>
              </div>
              <span className="aurum-poloska aurum-poloska-mal"><i style={{ transform: `scaleX(${dolyaShkaly(s.ping)})` }} /></span>
              {otkryta && <Protokoly servery={s.servery} />}
            </div>
          );
        })}
      </div>
    </aside>
  );
}
