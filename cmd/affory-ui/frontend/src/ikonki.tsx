import type { SVGProps } from "react";

// Интерфейсные значки одной рукой: штрих 1.6, скруглённые концы, viewBox 24.
// Эмодзи вместо значка не используется нигде, в том числе в пустом состоянии.

type P = SVGProps<SVGSVGElement>;

function Shtrih({ children, ...rest }: P) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...rest}
    >
      {children}
    </svg>
  );
}

export const IkPodklyuchenie = (p: P) => (
  <Shtrih {...p}><path d="M5 8.5a9.5 9.5 0 0114 0M8 12a5.5 5.5 0 018 0" /><circle cx="12" cy="17" r="1.6" fill="currentColor" stroke="none" /></Shtrih>
);
export const IkPravila = (p: P) => (
  <Shtrih {...p}><rect x="3.5" y="4" width="7" height="7" rx="1.6" /><rect x="13.5" y="4" width="7" height="4" rx="1.4" /><rect x="13.5" y="11" width="7" height="9" rx="1.6" /><rect x="3.5" y="14" width="7" height="6" rx="1.6" /></Shtrih>
);
export const IkNastroyki = (p: P) => (
  <Shtrih {...p}><circle cx="12" cy="12" r="3" /><path d="M19.4 14.2a1.5 1.5 0 00.3 1.65l.05.05a1.8 1.8 0 11-2.55 2.55l-.05-.05a1.5 1.5 0 00-1.65-.3 1.5 1.5 0 00-.9 1.37V19.7a1.8 1.8 0 11-3.6 0v-.1a1.5 1.5 0 00-.98-1.37 1.5 1.5 0 00-1.65.3l-.05.05a1.8 1.8 0 11-2.55-2.55l.05-.05a1.5 1.5 0 00.3-1.65 1.5 1.5 0 00-1.37-.9H4.3a1.8 1.8 0 110-3.6h.1a1.5 1.5 0 001.37-.98 1.5 1.5 0 00-.3-1.65l-.05-.05A1.8 1.8 0 117.97 4.6l.05.05a1.5 1.5 0 001.65.3h.07a1.5 1.5 0 00.9-1.37V3.5a1.8 1.8 0 113.6 0v.1a1.5 1.5 0 00.9 1.37 1.5 1.5 0 001.65-.3l.05-.05a1.8 1.8 0 112.55 2.55l-.05.05a1.5 1.5 0 00-.3 1.65v.07a1.5 1.5 0 001.37.9h.16a1.8 1.8 0 010 3.6h-.1a1.5 1.5 0 00-1.37.9z" /></Shtrih>
);
export const IkPoisk = (p: P) => (<Shtrih {...p}><circle cx="11" cy="11" r="6.4" /><path d="M20 20l-3.6-3.6" /></Shtrih>);
export const IkGalka = (p: P) => (<Shtrih strokeWidth={2.2} {...p}><path d="M5 12.6l4.6 4.6L19 7.4" /></Shtrih>);
export const IkStrelkaVniz = (p: P) => (<Shtrih {...p}><path d="M6 9.5l6 6 6-6" /></Shtrih>);
export const IkStrelkaVpravo = (p: P) => (<Shtrih {...p}><path d="M9.5 5l6.5 7-6.5 7" /></Shtrih>);
export const IkTreugolnik = (p: P) => (<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" {...p}><path d="M9 5.5l8 6.5-8 6.5z" /></svg>);
export const IkKopirovat = (p: P) => (<Shtrih {...p}><rect x="9" y="9" width="11" height="11" rx="2" /><path d="M15 6.5V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7a2 2 0 002 2h.5" /></Shtrih>);
export const IkShchit = (p: P) => (<Shtrih {...p}><path d="M12 3.5l7 2.8v5.1c0 4.2-2.9 7.6-7 9.1-4.1-1.5-7-4.9-7-9.1V6.3z" /></Shtrih>);
export const IkMonitor = (p: P) => (<Shtrih {...p}><rect x="3" y="4.5" width="18" height="12" rx="2" /><path d="M9 20h6M12 16.5V20" /></Shtrih>);
export const IkPusk = (p: P) => (<Shtrih {...p}><path d="M8 5.5l10 6.5-10 6.5z" /></Shtrih>);
export const IkProksi = (p: P) => (<Shtrih {...p}><circle cx="12" cy="6" r="2.5" /><circle cx="5.5" cy="18" r="2.5" /><circle cx="18.5" cy="18" r="2.5" /><path d="M12 8.5v3.2M12 11.7L6.6 15.9M12 11.7l5.4 4.2" /></Shtrih>);
export const IkObnovit = (p: P) => (<Shtrih {...p}><path d="M20 12a8 8 0 11-2.6-5.9" /><path d="M20.2 4.6v4.2H16" /></Shtrih>);
export const IkArhiv = (p: P) => (<Shtrih {...p}><rect x="4" y="4" width="16" height="16" rx="2.2" /><path d="M11 4v6l1.5-1.3L14 10V4" /></Shtrih>);
export const IkZakrepit = (p: P) => (<Shtrih {...p}><path d="M8 3h8l-1 7 3 3v2H6v-2l3-3-1-7zM12 15v6" /></Shtrih>);
export const IkServer = (p: P) => (<Shtrih {...p}><rect x="3.8" y="4.5" width="16.4" height="6" rx="1.8" /><rect x="3.8" y="13.5" width="16.4" height="6" rx="1.8" /><path d="M7.3 7.5h.01M7.3 16.5h.01" /></Shtrih>);
export const IkVnimanie = (p: P) => (<Shtrih strokeWidth={1.8} {...p}><circle cx="12" cy="12" r="8.4" /><path d="M12 7.8v5M12 16.2h.01" /></Shtrih>);
export const IkPitanie = (p: P) => (<Shtrih {...p}><path d="M12 3.8v7.4" /><path d="M17.2 6.6a7 7 0 11-10.4 0" /></Shtrih>);
export const IkPlyus = (p: P) => (<Shtrih strokeWidth={1.9} {...p}><path d="M12 5.5v13M5.5 12h13" /></Shtrih>);
export const IkTochki = (p: P) => (<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" {...p}><circle cx="6" cy="12" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="18" cy="12" r="1.6" /></svg>);
export const IkKorzina = (p: P) => (<Shtrih {...p}><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" /></Shtrih>);
export const IkKarandash = (p: P) => (<Shtrih {...p}><path d="M4 20l.9-3.8L16.1 5a2.1 2.1 0 013 3L7.9 19.2z" /><path d="M14.4 6.7l3 3" /></Shtrih>);
export const IkPapka = (p: P) => (<Shtrih {...p}><path d="M4 7.5A2 2 0 016 5.5h3.4l2 2.4H18a2 2 0 012 2v7.6a2 2 0 01-2 2H6a2 2 0 01-2-2z" /></Shtrih>);
export const IkSayt = (p: P) => (<Shtrih {...p}><circle cx="12" cy="12" r="8.4" /><path d="M3.8 12h16.4M12 3.6a13 13 0 010 16.8 13 13 0 010-16.8" /></Shtrih>);
export const IkSsylka = (p: P) => (<Shtrih {...p}><path d="M10.4 13.6a3.6 3.6 0 005.4.4l2.6-2.6a3.6 3.6 0 00-5.1-5.1l-1.5 1.5" /><path d="M13.6 10.4a3.6 3.6 0 00-5.4-.4l-2.6 2.6a3.6 3.6 0 005.1 5.1l1.5-1.5" /></Shtrih>);
export const IkVniz = (p: P) => (<Shtrih {...p}><path d="M12 4.5v12M6.6 11.4L12 16.8l5.4-5.4" /></Shtrih>);
export const IkVverh = (p: P) => (<Shtrih {...p}><path d="M12 19.5v-12M6.6 12.6L12 7.2l5.4 5.4" /></Shtrih>);
export const IkDiagnostika = (p: P) => (<Shtrih {...p}><path d="M3.5 12.5h4l2.2-5.6 3.4 10.2 2.2-4.6h5.2" /></Shtrih>);
export const IkProfil = (p: P) => (<Shtrih {...p}><circle cx="12" cy="8.5" r="3.6" /><path d="M5 19.4a7.2 7.2 0 0114 0" /></Shtrih>);
export const IkSvernut = (p: P) => (<Shtrih strokeWidth={1.3} {...p}><path d="M6 12h12" /></Shtrih>);
export const IkRazvernut = (p: P) => (<Shtrih strokeWidth={1.3} {...p}><rect x="6.5" y="6.5" width="11" height="11" rx="1.4" /></Shtrih>);
export const IkZakryt = (p: P) => (<Shtrih strokeWidth={1.3} {...p}><path d="M6.5 6.5l11 11M17.5 6.5l-11 11" /></Shtrih>);
