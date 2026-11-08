// Значки AURUM: один штрих, 1.7, скруглённые концы, как на референсе.
import type { SVGProps } from "react";
const P = (p: SVGProps<SVGSVGElement>) => ({ width: 22, height: 22, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: 1.7, strokeLinecap: "round" as const, strokeLinejoin: "round" as const, "aria-hidden": true, ...p });

export const IkDerevo = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P(p)}><circle cx="12" cy="5.5" r="2.3" /><circle cx="5.5" cy="18" r="2.3" /><circle cx="18.5" cy="18" r="2.3" /><path d="M12 7.8v4.2M12 12c0 0-6.5.2-6.5 3.7M12 12c0 0 6.5.2 6.5 3.7" /></svg>
);
export const IkKarta = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P(p)}><path d="M3.5 6.2 9 4l6 2.2 5.5-2.2v13.8L15 20l-6-2.2-5.5 2.2z" /><path d="M9 4v13.8M15 6.2V20" /></svg>
);
export const IkVniz = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P({ width: 16, height: 16, ...p })}><path d="m6 9.5 6 6 6-6" /></svg>
);
export const IkTochki = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P({ width: 18, height: 18, ...p })}><rect x="4" y="5" width="16" height="14" rx="3.5" /><path d="M4 9.5h16" /></svg>
);
export const IkPilyulya = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P({ width: 18, height: 18, ...p })}><rect x="3.5" y="8" width="17" height="8" rx="4" /><circle cx="8" cy="12" r="1.6" /></svg>
);
export const IkPapka = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P({ width: 15, height: 15, ...p })}><path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2.2h7a2 2 0 0 1 2 2v7.3a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" /></svg>
);
export const IkZakryt = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P({ width: 16, height: 16, ...p })}><path d="m6 6 12 12M18 6 6 18" /></svg>
);
export const IkPoisk = (p: SVGProps<SVGSVGElement>) => (
  <svg {...P({ width: 16, height: 16, ...p })}><circle cx="11" cy="11" r="6.2" /><path d="m20 20-4.2-4.2" /></svg>
);
