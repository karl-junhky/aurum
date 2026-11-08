// Общие элементы оболочки в духе macOS: переключатель и всплывающий список.
import { useEffect, useRef, useState } from "react";
import { IkVniz } from "./znachki";

export function Tumbler({ on, onChange, label }: { on: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <button type="button" role="switch" aria-checked={on} aria-label={label} className="aurum-switch" onClick={() => onChange(!on)}>
      <i />
    </button>
  );
}

export function Vypadayushchiy<T extends string | number>({ znachenie, varianty, naVybor, ariaLabel }: {
  znachenie: T;
  varianty: { z: T; podpis: string }[];
  naVybor: (v: T) => void;
  ariaLabel: string;
}) {
  const [otkryt, setOtkryt] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const tek = varianty.find((x) => x.z === znachenie) ?? varianty[0];
  useEffect(() => {
    if (!otkryt) return;
    const out = (e: PointerEvent) => { if (!ref.current?.contains(e.target as Node)) setOtkryt(false); };
    window.addEventListener("pointerdown", out);
    return () => window.removeEventListener("pointerdown", out);
  }, [otkryt]);
  return (
    <div className="aurum-select-wrap" ref={ref}>
      <button type="button" className="aurum-select-btn" aria-haspopup="listbox" aria-expanded={otkryt} aria-label={ariaLabel} onClick={() => setOtkryt((v) => !v)}>
        <span>{tek.podpis}</span><IkVniz />
      </button>
      {otkryt && (
        <ul className="aurum-popover aurum-select-list" role="listbox" aria-label={ariaLabel}>
          {varianty.map((v) => (
            <li key={String(v.z)}>
              <button type="button" role="option" aria-selected={v.z === znachenie} className={v.z === znachenie ? "is-on" : ""} onClick={() => { naVybor(v.z); setOtkryt(false); }}>
                <span className="aurum-galka">{v.z === znachenie ? "✓" : ""}</span><span>{v.podpis}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
