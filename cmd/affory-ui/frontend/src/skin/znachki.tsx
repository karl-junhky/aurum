// Значки AURUM: один штрих 1.75, скруглённые концы, как SF Symbols.
import type { SVGProps } from "react";

const P = (p: SVGProps<SVGSVGElement>) => ({
  width: 20, height: 20, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: 1.75,
  strokeLinecap: "round" as const, strokeLinejoin: "round" as const, "aria-hidden": true, ...p,
});
const M = (w: number) => (p: SVGProps<SVGSVGElement>) => ({ ...P({ width: w, height: w, ...p }) });

/** Дерево серверов (левая круглая кнопка). */
export const IkDerevo = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P(p)}><rect x="9.5" y="3.5" width="5" height="4.5" rx="1.2" /><rect x="3.5" y="15.5" width="5" height="4.5" rx="1.2" /><rect x="15.5" y="15.5" width="5" height="4.5" rx="1.2" /><path d="M12 8v4m-6 3.5V13a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v2.5" /></svg>
);
/** Карта (правая круглая кнопка). */
export const IkKarta = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P(p)}><path d="M3.5 6.2 9 4l6 2.2 5.5-2.2v13.8L15 20l-6-2.2-5.5 2.2z" /><path d="M9 4v13.8M15 6.2V20" /></svg>
);
export const IkVniz = (p: SVGProps<SVGSVGElement>) => <svg {...M(15)(p)}><path d="m6 9.5 6 6 6-6" /></svg>;
export const IkShesterenka = (p: SVGProps<SVGSVGElement>) => (
  <svg {...M(17)(p)}><circle cx="12" cy="12" r="3.2" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9c.26.6.77 1.02 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>
);
export const IkPlyus = (p: SVGProps<SVGSVGElement>) => <svg {...M(16)(p)}><path d="M12 5v14M5 12h14" /></svg>;
export const IkObnovit = (p: SVGProps<SVGSVGElement>) => <svg {...M(15)(p)}><path d="M20 11a8.1 8.1 0 0 0-15.5-2m-.5-4v4h4" /><path d="M4 13a8.1 8.1 0 0 0 15.5 2m.5 4v-4h-4" /></svg>;
export const IkIskry = (p: SVGProps<SVGSVGElement>) => <svg {...M(15)(p)}><path d="m12 3 1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9z" /><path d="M5 3v3M3.5 4.5h3" /></svg>;
export const IkMolniya = (p: SVGProps<SVGSVGElement>) => <svg {...M(14)(p)}><path d="M13 2.5 5 13.5h6l-1 8 8-11h-6z" /></svg>;
export const IkTochki = (p: SVGProps<SVGSVGElement>) => <svg {...M(16)(p)}><rect x="3.5" y="4.5" width="17" height="15" rx="3" /><path d="M3.5 9h17" /></svg>;
export const IkPilyulya = (p: SVGProps<SVGSVGElement>) => <svg {...M(16)(p)}><rect x="3.5" y="7.5" width="17" height="9" rx="4.5" /><circle cx="8" cy="12" r="1.8" /></svg>;
export const IkPapka = (p: SVGProps<SVGSVGElement>) => <svg {...M(15)(p)}><path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2.2h7a2 2 0 0 1 2 2v7.3a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" /></svg>;
export const IkZakryt = (p: SVGProps<SVGSVGElement>) => <svg {...M(15)(p)}><path d="m6 6 12 12M18 6 6 18" /></svg>;
export const IkPoisk = (p: SVGProps<SVGSVGElement>) => <svg {...M(15)(p)}><circle cx="11" cy="11" r="6.2" /><path d="m20 20-4.2-4.2" /></svg>;
export const IkPlaneta = (p: SVGProps<SVGSVGElement>) => <svg {...M(16)(p)}><circle cx="12" cy="12" r="8.5" /><path d="M3.8 12h16.4M12 3.5c2.6 2.4 3.8 5.2 3.8 8.5s-1.2 6.1-3.8 8.5c-2.6-2.4-3.8-5.2-3.8-8.5S9.4 5.9 12 3.5z" /></svg>;
export const IkPravila = (p: SVGProps<SVGSVGElement>) => <svg {...M(16)(p)}><path d="M4 7h10M18 7h2M4 17h2M10 17h10" /><circle cx="16" cy="7" r="2" /><circle cx="8" cy="17" r="2" /></svg>;
