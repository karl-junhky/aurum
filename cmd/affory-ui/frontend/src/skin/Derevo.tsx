// Боковая стеклянная панель серверов, как на первом кадре референса: группы
// со стрелкой, внутри строки с флагом, протоколом и пингом цветом.
import { useState } from "react";
import { Flag } from "./flagi";
import type { Gruppa } from "./dom";
import { IkPoisk, IkVniz, IkZakryt } from "./znachki";

function ton(ping: string): "ok" | "mid" | "bad" | "" {
  const n = parseInt(ping, 10);
  if (Number.isNaN(n)) return "";
  return n < 60 ? "ok" : n < 150 ? "mid" : "bad";
}

export function Derevo({ gruppy, zakryt, versiya }: { gruppy: Gruppa[]; zakryt: () => void; versiya: string }) {
  const [svernuty, setSvernuty] = useState<string[]>([]);
  const [zapros, setZapros] = useState("");
  const z = zapros.trim().toLowerCase();
  return (
    <aside className="aurum-derevo" aria-label="Серверы" data-testid="aurum-derevo-panel">
      <header>
        <div>
          <b className="aurum-slovo">AURUM</b>
          <small>{versiya ? `ядро Affory ${versiya}` : "ядро Affory"}</small>
        </div>
        <button type="button" onClick={zakryt} aria-label="Закрыть список" className="aurum-ik"><IkZakryt /></button>
      </header>
      <label className="aurum-poisk">
        <IkPoisk />
        <input value={zapros} onChange={(e) => setZapros(e.target.value)} placeholder="Найти сервер" aria-label="Найти сервер" />
      </label>
      <div className="aurum-derevo-spisok">
        {gruppy.length === 0 && <p className="aurum-pusto">Серверов пока нет. Добавь подписку в полном окне.</p>}
        {gruppy.map((g) => {
          const stroki = g.servery.filter((s) => !z || `${s.imya} ${s.transport}`.toLowerCase().includes(z));
          if (z && !stroki.length) return null;
          const zakr = svernuty.includes(g.imya) && !z;
          return (
            <section key={g.imya}>
              <button type="button" className="aurum-gruppa" aria-expanded={!zakr} onClick={() => setSvernuty((s) => (s.includes(g.imya) ? s.filter((x) => x !== g.imya) : [...s, g.imya]))}>
                <span>{g.imya}</span><i>{g.servery.length}</i><IkVniz className={zakr ? "is-zakr" : ""} />
              </button>
              {!zakr && (
                <ul>
                  {stroki.map((s) => (
                    <li key={s.imya + s.transport}>
                      <button type="button" className={s.aktiven ? "is-on" : ""} onClick={s.nazhat} aria-pressed={s.aktiven}>
                        <Flag imya={s.imya} razmer={26} />
                        <span className="aurum-derevo-imya"><b>{s.imya}</b><small>{s.transport}</small></span>
                        <span className="aurum-ping" data-ton={ton(s.ping)}>{s.ping}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          );
        })}
      </div>
    </aside>
  );
}
