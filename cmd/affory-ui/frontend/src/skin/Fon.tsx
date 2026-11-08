// Живые фоны AURUM. Всё рисуется на canvas и WebGL без единого файла и без
// сети: переливающийся шёлк, планета с метками серверов (крутится мышью),
// галактика, цветные изгибы, сияние. Плюс своё видео или картинка.
// Фон лежит под #root; события ловит только планета и только в режиме пилюли.
import { useEffect, useRef, useState } from "react";
import { useUstanovki, type Fon as VidFona, type Palitra } from "./ustanovki";
import { SOBYTIE_METKI, zhivoe } from "./store";

type Ris = (t: number, dt: number) => void;
interface Kontekst {
  c: CanvasRenderingContext2D;
  cv: HTMLCanvasElement;
  w: number;
  h: number;
  rgb: [number, number, number];
  mysh: { x: number; y: number };
  ochistka: (() => void)[];
}

const TAU = Math.PI * 2;
const rgba = (rgb: number[], a: number) => `rgba(${rgb[0]},${rgb[1]},${rgb[2]},${a})`;

// ---------- Планета ----------
// Грубые контуры материков в градусах (долгота, широта). Нужны не для
// географии, а чтобы точечная сфера читалась как Земля.
const MATERIKI: [number, number][][] = [
  [[-168,66],[-155,71],[-130,70],[-95,69],[-85,70],[-80,63],[-65,60],[-56,52],[-66,45],[-76,38],[-81,31],[-80,25],[-88,30],[-97,28],[-97,20],[-90,21],[-87,16],[-83,9],[-78,8],[-86,13],[-92,15],[-105,20],[-112,29],[-115,31],[-123,38],[-125,48],[-135,58],[-150,60],[-165,60]],
  [[-78,8],[-62,11],[-52,5],[-35,-6],[-39,-15],[-48,-26],[-58,-38],[-65,-42],[-68,-53],[-74,-50],[-73,-37],[-71,-18],[-81,-6],[-79,2]],
  [[-55,60],[-45,60],[-20,70],[-20,82],[-45,83],[-70,78],[-58,70]],
  [[-10,36],[-9,43],[-2,47],[-5,48],[2,51],[8,54],[10,58],[5,62],[15,69],[30,71],[60,69],[80,73],[105,78],[140,72],[160,70],[180,68],[180,65],[165,60],[155,58],[142,52],[140,45],[130,42],[122,40],[121,31],[110,20],[106,10],[100,13],[98,8],[104,1],[98,16],[92,22],[88,22],[80,15],[77,8],[73,20],[67,25],[57,26],[50,30],[48,29],[43,13],[35,28],[34,31],[36,36],[28,36],[26,40],[22,37],[18,40],[14,45],[12,44],[16,38],[8,44],[3,43],[-2,37]],
  [[-17,21],[-10,30],[-6,36],[10,37],[20,32],[32,31],[35,28],[43,12],[51,12],[40,-3],[40,-15],[35,-25],[27,-34],[18,-34],[12,-18],[9,4],[-8,4],[-17,14]],
  [[114,-22],[122,-18],[130,-12],[137,-12],[142,-11],[146,-19],[153,-26],[150,-37],[140,-38],[131,-31],[115,-34]],
  [[-5,50],[2,51],[0,58],[-6,58],[-10,54]],
  [[130,31],[141,36],[142,44],[140,41],[133,34]],
  [[95,5],[106,-6],[115,-8],[120,-1],[109,2]],
  [[43,-13],[50,-15],[47,-25],[44,-22]],
  [[166,-46],[174,-41],[178,-38],[170,-46]],
];

let maskaKesh: Uint8ClampedArray | null = null;
const MW = 360, MH = 180;
function maska(): Uint8ClampedArray {
  if (maskaKesh) return maskaKesh;
  const c = document.createElement("canvas");
  c.width = MW; c.height = MH;
  const g = c.getContext("2d");
  if (!g) return (maskaKesh = new Uint8ClampedArray(MW * MH * 4));
  g.fillStyle = "#fff";
  for (const p of MATERIKI) {
    g.beginPath();
    p.forEach(([lo, la], i) => (i ? g.lineTo(lo + 180, 90 - la) : g.moveTo(lo + 180, 90 - la)));
    g.closePath();
    g.fill();
  }
  maskaKesh = g.getImageData(0, 0, MW, MH).data;
  return maskaKesh;
}

const rad = (g: number) => (g * Math.PI) / 180;
/** Кратчайшая разница углов в (-π, π]. */
const dUgol = (a: number, b: number) => ((((b - a) % TAU) + TAU + Math.PI) % TAU) - Math.PI;

function vek(lat: number, lon: number): [number, number, number] {
  const la = rad(lat), lo = rad(lon);
  return [Math.cos(la) * Math.sin(lo), Math.sin(la), Math.cos(la) * Math.cos(lo)];
}

function planeta(k: Kontekst): Ris {
  const m = maska();
  const N = 9000;
  const zol = Math.PI * (3 - Math.sqrt(5));
  const sx: number[] = [], sy: number[] = [], sz: number[] = [];
  const ox: number[] = [], oy: number[] = [], oz: number[] = [];
  for (let i = 0; i < N; i++) {
    const y = 1 - (i / (N - 1)) * 2;
    const r = Math.sqrt(1 - y * y);
    const x = Math.cos(zol * i) * r, z = Math.sin(zol * i) * r;
    const px = Math.min(MW - 1, Math.max(0, Math.floor(Math.atan2(x, z) * (180 / Math.PI) + 180)));
    const py = Math.min(MH - 1, Math.max(0, Math.floor(90 - Math.asin(y) * (180 / Math.PI))));
    if (m[(py * MW + px) * 4 + 3] > 100) { sx.push(x); sy.push(y); sz.push(z); }
    else if (i % 3 === 0) { ox.push(x); oy.push(y); oz.push(z); }
  }
  const BAKETOV = 5;
  const bx = Array.from({ length: BAKETOV }, () => new Float32Array(sx.length));
  const by = Array.from({ length: BAKETOV }, () => new Float32Array(sx.length));
  const nb = new Int32Array(BAKETOV);

  // дом: долгота по часовому поясу, чтобы дуга шла «от тебя» без геолокации
  const domLon = -new Date().getTimezoneOffset() / 4;
  const dom = vek(50, domLon);

  const st = { ug: -rad(domLon) * 0.6, tl: 0.42, vug: 0, vtl: 0, zoom: 1, drag: false, lx: 0, ly: 0, sdvig: 0, prostoy: 0, cel: null as null | { ug: number; tl: number }, fokus: zhivoe().fokus, nad: null as null | string, nadX: 0, nadY: 0 };
  const aktivnaya = zhivoe().metki.find((x) => x.aktivna);
  if (aktivnaya) st.cel = { ug: -rad(aktivnaya.lon), tl: rad(aktivnaya.lat) * 0.7 };

  const cv = k.cv;
  const lokalno = (e: PointerEvent | WheelEvent) => { const r = cv.getBoundingClientRect(); return { x: e.clientX - r.left, y: e.clientY - r.top }; };
  const pd = (e: PointerEvent) => { st.drag = true; st.sdvig = 0; st.lx = e.clientX; st.ly = e.clientY; st.cel = null; st.vug = st.vtl = 0; cv.setPointerCapture(e.pointerId); };
  const pm = (e: PointerEvent) => {
    const p = lokalno(e);
    st.nadX = p.x; st.nadY = p.y;
    if (!st.drag) return;
    const dx = e.clientX - st.lx, dy = e.clientY - st.ly;
    st.lx = e.clientX; st.ly = e.clientY; st.sdvig += Math.abs(dx) + Math.abs(dy);
    const kf = 0.0052 / st.zoom;
    st.ug += dx * kf; st.tl = Math.max(-1.25, Math.min(1.25, st.tl + dy * kf));
    st.vug = dx * kf; st.vtl = dy * kf;
    st.prostoy = 0;
  };
  const pu = (e: PointerEvent) => {
    const bylClick = st.sdvig < 5;
    st.drag = false;
    try { cv.releasePointerCapture(e.pointerId); } catch { /* уже отпущен */ }
    if (bylClick && st.nad) window.dispatchEvent(new CustomEvent(SOBYTIE_METKI, { detail: { klyuch: st.nad } }));
  };
  const wh = (e: WheelEvent) => { e.preventDefault(); st.zoom = Math.max(0.7, Math.min(1.9, st.zoom * Math.exp(-e.deltaY * 0.0012))); };
  const dbl = () => { st.cel = { ug: -rad(domLon) * 0.6, tl: 0.42 }; st.zoom = 1; };
  cv.addEventListener("pointerdown", pd); cv.addEventListener("pointermove", pm); cv.addEventListener("pointerup", pu); cv.addEventListener("pointercancel", pu);
  cv.addEventListener("wheel", wh, { passive: false }); cv.addEventListener("dblclick", dbl);
  k.ochistka.push(() => { cv.removeEventListener("pointerdown", pd); cv.removeEventListener("pointermove", pm); cv.removeEventListener("pointerup", pu); cv.removeEventListener("pointercancel", pu); cv.removeEventListener("wheel", wh); cv.removeEventListener("dblclick", dbl); });

  let t0 = 0, kadrov = 0, kolPravo = 0;
  return (t, dt) => {
    const { c, w, h, rgb } = k;
    c.clearRect(0, 0, w, h);
    const zh = zhivoe();
    if (zh.fokus !== st.fokus) {
      st.fokus = zh.fokus;
      const mt = zh.metki.find((x) => x.kod === zh.fokusKod);
      if (mt) { st.cel = { ug: -rad(mt.lon), tl: rad(mt.lat) * 0.7 }; st.drag = false; }
    }
    // движение: цель, инерция, медленное вращение в покое
    if (st.cel) {
      const du = dUgol(st.ug, st.cel.ug), dtl = st.cel.tl - st.tl;
      const e = 1 - Math.pow(0.0009, dt / 1000);
      st.ug += du * e; st.tl += dtl * e;
      if (Math.abs(du) < 0.002 && Math.abs(dtl) < 0.002) st.cel = null;
    } else if (!st.drag) {
      st.ug += st.vug; st.tl = Math.max(-1.25, Math.min(1.25, st.tl + st.vtl));
      const g = Math.pow(0.02, dt / 1000); st.vug *= g; st.vtl *= g;
      st.prostoy += dt;
      if (st.prostoy > 5000 && Math.abs(st.vug) < 0.0003) st.ug += dt * 0.00004;
    }
    // планета занимает свободное место справа от колонки управления; если
    // его мало, колесо мыши увеличивает её и она уходит за стекло
    const smeshchen = document.documentElement.dataset.gud === "1" && w > 900;
    if (smeshchen && (kadrov++ % 20 === 0)) {
      const kol = document.querySelector(".aurum-gud-kolonka");
      kolPravo = kol ? kol.getBoundingClientRect().right + 20 : w * 0.45;
    }
    let cx = w * 0.5, R = Math.min(w * 0.4, h * 0.43) * st.zoom;
    const cy = h * 0.5;
    if (smeshchen) {
      const svobodno = w - 20 - kolPravo;
      const R0 = Math.max(150, Math.min(svobodno / 2, h * 0.43));
      R = R0 * st.zoom;
      cx = Math.max(kolPravo + R0, w - 20 - R0);
    }
    const ca = Math.cos(st.ug), sa = Math.sin(st.ug), ct = Math.cos(st.tl), sn = Math.sin(st.tl);
    const proekt = (x: number, y: number, z: number): [number, number, number] => {
      const x1 = x * ca + z * sa, z1 = -x * sa + z * ca;
      return [cx + x1 * R, cy - (y * ct - z1 * sn) * R, y * sn + z1 * ct];
    };

    // ореол и стеклянное тело
    const halo = c.createRadialGradient(cx, cy, R * 0.82, cx, cy, R * 1.32);
    halo.addColorStop(0, rgba(rgb, 0)); halo.addColorStop(0.55, rgba(rgb, 0.11)); halo.addColorStop(1, rgba(rgb, 0));
    c.fillStyle = halo; c.fillRect(cx - R * 1.4, cy - R * 1.4, R * 2.8, R * 2.8);
    const body = c.createRadialGradient(cx - R * 0.35, cy - R * 0.4, R * 0.05, cx, cy, R);
    body.addColorStop(0, "rgba(40,46,64,0.78)"); body.addColorStop(0.75, "rgba(12,14,22,0.88)"); body.addColorStop(1, "rgba(6,7,12,0.95)");
    c.beginPath(); c.arc(cx, cy, R, 0, TAU); c.fillStyle = body; c.fill();
    c.lineWidth = 1.2; c.strokeStyle = "rgba(255,255,255,0.2)"; c.stroke();

    // океанская пыль: одним проходом
    c.fillStyle = "rgba(190,205,230,0.10)"; c.beginPath();
    for (let i = 0; i < ox.length; i++) { const [px, py, pz] = proekt(ox[i], oy[i], oz[i]); if (pz > 0.02) c.rect(px - 0.5, py - 0.5, 1, 1); }
    c.fill();
    // суша: пять корзин яркости, по одной заливке на корзину
    nb.fill(0);
    for (let i = 0; i < sx.length; i++) {
      const [px, py, pz] = proekt(sx[i], sy[i], sz[i]);
      if (pz <= 0.02) continue;
      const b = Math.min(BAKETOV - 1, Math.floor(pz * BAKETOV));
      bx[b][nb[b]] = px; by[b][nb[b]] = py; nb[b]++;
    }
    for (let b = 0; b < BAKETOV; b++) {
      const zz = (b + 0.5) / BAKETOV, s = 1.1 + 1.1 * zz;
      c.fillStyle = `rgba(244,246,252,${0.30 + 0.70 * Math.pow(zz, 0.65)})`; c.beginPath();
      for (let i = 0; i < nb[b]; i++) c.rect(bx[b][i] - s / 2, by[b][i] - s / 2, s, s);
      c.fill();
    }

    // дуга от дома до выбранной страны
    const akt = zh.metki.find((x) => x.aktivna);
    if (akt) {
      const a = dom, b = vek(akt.lat, akt.lon);
      const om = Math.acos(Math.max(-1, Math.min(1, a[0] * b[0] + a[1] * b[1] + a[2] * b[2])));
      if (om > 0.02) {
        c.save(); c.lineWidth = 1.6; c.lineCap = "round"; c.setLineDash([2, 7]); c.lineDashOffset = -t * 0.02;
        c.strokeStyle = rgba(rgb, 0.85); c.shadowColor = rgba(rgb, 0.8); c.shadowBlur = 8; c.beginPath();
        let pen = false;
        const ST = 48;
        for (let i = 0; i <= ST; i++) {
          const f = i / ST, s1 = Math.sin((1 - f) * om) / Math.sin(om), s2 = Math.sin(f * om) / Math.sin(om), lift = 1 + 0.22 * Math.sin(Math.PI * f);
          const [px, py, pz] = proekt((a[0] * s1 + b[0] * s2) * lift, (a[1] * s1 + b[1] * s2) * lift, (a[2] * s1 + b[2] * s2) * lift);
          if (pz > -0.05) { if (pen) c.lineTo(px, py); else c.moveTo(px, py); pen = true; } else pen = false;
        }
        c.stroke(); c.restore();
        // бегущая искра
        const f = (t0 + t * 0.00022) % 1, s1 = Math.sin((1 - f) * om) / Math.sin(om), s2 = Math.sin(f * om) / Math.sin(om), lift = 1 + 0.22 * Math.sin(Math.PI * f);
        const [px, py, pz] = proekt((a[0] * s1 + b[0] * s2) * lift, (a[1] * s1 + b[1] * s2) * lift, (a[2] * s1 + b[2] * s2) * lift);
        if (pz > 0) { c.fillStyle = "#fff"; c.shadowColor = rgba(rgb, 1); c.shadowBlur = 12; c.beginPath(); c.arc(px, py, 2.4, 0, TAU); c.fill(); c.shadowBlur = 0; }
      }
      const [hx, hy, hz] = proekt(dom[0], dom[1], dom[2]);
      if (hz > 0) { c.fillStyle = "rgba(255,255,255,0.9)"; c.beginPath(); c.arc(hx, hy, 3, 0, TAU); c.fill(); }
    }

    // метки серверов
    let nad: string | null = null;
    const tonRgb: Record<string, string> = { ok: "95,224,138", mid: "242,195,94", bad: "255,122,122", "": "190,200,220" };
    const risLabel: (() => void)[] = [];
    for (const mt of zh.metki) {
      const [px, py, pz] = proekt(...vek(mt.lat, mt.lon));
      if (pz <= 0.08) continue;
      const tc = tonRgb[mt.ton];
      const pod = !st.drag && Math.hypot(px - st.nadX, py - st.nadY) < 14;
      if (pod) nad = mt.klyuch;
      const puls = mt.aktivna ? 0.5 + 0.5 * Math.sin(t * 0.004) : 0;
      if (mt.aktivna) { c.beginPath(); c.arc(px, py, 9 + puls * 8, 0, TAU); c.strokeStyle = rgba(rgb, 0.55 * (1 - puls)); c.lineWidth = 1.5; c.stroke(); }
      c.beginPath(); c.arc(px, py, mt.aktivna || pod ? 5.2 : 3.6, 0, TAU);
      c.fillStyle = `rgb(${tc})`; c.shadowColor = `rgb(${tc})`; c.shadowBlur = mt.aktivna || pod ? 14 : 6; c.fill(); c.shadowBlur = 0;
      c.lineWidth = 1.4; c.strokeStyle = "rgba(255,255,255,0.9)"; c.stroke();
      if (mt.aktivna || pod) {
        const tekst = `${mt.imya}${mt.ping !== null ? ` · ${mt.ping} мс` : ""}`;
        risLabel.push(() => {
          c.font = '600 12.5px -apple-system, "SF Pro Text", "Segoe UI Variable Text", Inter, system-ui, sans-serif';
          const tw = c.measureText(tekst).width + 20, lx = px + 12, ly = py - 12;
          c.fillStyle = "rgba(14,16,24,0.82)"; c.beginPath(); c.roundRect(lx, ly - 13, tw, 26, 13); c.fill();
          c.strokeStyle = "rgba(255,255,255,0.16)"; c.lineWidth = 1; c.stroke();
          c.fillStyle = "#f5f6fa"; c.textBaseline = "middle"; c.fillText(tekst, lx + 10, ly);
        });
      }
    }
    risLabel.forEach((f) => f());
    st.nad = nad;
    cv.style.cursor = nad ? "pointer" : st.drag ? "grabbing" : "grab";
  };
}

// ---------- Галактика ----------
function galaktika(k: Kontekst): Ris {
  const N = 5200;
  const z = Array.from({ length: N }, (_, i) => {
    const r = Math.pow(Math.random(), 0.62);
    const ugl = (i % 3) * (TAU / 3) + r * 7.2 + (Math.random() - 0.5) * (0.32 + (1 - r) * 0.4);
    return { r, ugl, s: 0.6 + Math.random() * 1.7, f: Math.random() * TAU, t: Math.random() };
  });
  const fon = Array.from({ length: 140 }, () => ({ x: Math.random(), y: Math.random(), s: Math.random() * 1.2 + 0.2, f: Math.random() * TAU }));
  let phi = 0;
  return (t, dt) => {
    const { c, w, h, rgb } = k;
    c.clearRect(0, 0, w, h);
    phi += dt * 0.00004;
    for (const s of fon) { c.fillStyle = `rgba(255,255,255,${0.15 + 0.25 * (0.5 + 0.5 * Math.sin(t * 0.0006 + s.f))})`; c.fillRect(s.x * w, s.y * h, s.s, s.s); }
    const cx = w * (0.5 + (k.mysh.x - 0.5) * 0.04), cy = h * (0.5 + (k.mysh.y - 0.5) * 0.04);
    const R = Math.min(w * 0.62, h * 0.95);
    const yadro = c.createRadialGradient(cx, cy, 0, cx, cy, R * 0.34);
    yadro.addColorStop(0, "rgba(255,244,222,0.55)"); yadro.addColorStop(0.35, rgba(rgb, 0.22)); yadro.addColorStop(1, rgba(rgb, 0));
    c.fillStyle = yadro; c.fillRect(cx - R, cy - R, R * 2, R * 2);
    c.globalCompositeOperation = "lighter";
    for (const s of z) {
      const a = s.ugl + phi * (1.6 - s.r);
      const tw = 0.55 + 0.45 * Math.sin(t * 0.002 + s.f);
      const ton = s.r < 0.25 ? "255,236,200" : s.t > 0.8 ? "170,200,255" : `${rgb[0]},${rgb[1]},${rgb[2]}`;
      c.fillStyle = `rgba(${ton},${Math.min(0.9, (0.38 + (1 - s.r) * 0.6) * tw)})`;
      c.fillRect(cx + Math.cos(a) * s.r * R, cy + Math.sin(a) * s.r * R * 0.52, s.s, s.s);
    }
    c.globalCompositeOperation = "source-over";
  };
}

function sdvigOttenka([r, g, b]: number[], grad: number): [number, number, number] {
  const a = (grad * Math.PI) / 180, cs = Math.cos(a), sn = Math.sin(a);
  const Y = 0.299 * r + 0.587 * g + 0.114 * b, I = 0.596 * r - 0.274 * g - 0.322 * b, Q = 0.211 * r - 0.523 * g + 0.312 * b;
  const I2 = I * cs - Q * sn, Q2 = I * sn + Q * cs;
  const cl = (v: number) => Math.max(0, Math.min(255, Math.round(v)));
  return [cl(Y + 0.956 * I2 + 0.621 * Q2), cl(Y - 0.272 * I2 - 0.647 * Q2), cl(Y - 1.106 * I2 + 1.703 * Q2)];
}

// ---------- Цветные изгибы (запасной фон, если нет WebGL) ----------
function izgib(k: Kontekst): Ris {
  const ottenki = [0, 40, -35, 90, 160];
  return (t) => {
    const { c, w, h, rgb } = k;
    c.clearRect(0, 0, w, h);
    c.globalCompositeOperation = "lighter";
    ottenki.forEach((sdv, i) => {
      const x = w * (0.5 + 0.42 * Math.sin(t * 0.00018 * (1 + i * 0.23) + i * 1.7) + (k.mysh.x - 0.5) * 0.06);
      const y = h * (0.5 + 0.38 * Math.cos(t * 0.00014 * (1 + i * 0.31) + i * 2.3));
      const r = Math.max(w, h) * (0.34 + 0.07 * Math.sin(t * 0.0003 + i));
      const [rr, gg, bb] = sdvigOttenka(rgb, sdv);
      const g = c.createRadialGradient(x, y, 0, x, y, r);
      g.addColorStop(0, `rgba(${rr},${gg},${bb},0.55)`); g.addColorStop(1, `rgba(${rr},${gg},${bb},0)`);
      c.fillStyle = g; c.fillRect(0, 0, w, h);
    });
    c.globalCompositeOperation = "source-over";
  };
}

// ---------- Сияние ----------
function gradient(k: Kontekst): Ris {
  return (t) => {
    const { c, w, h, rgb } = k;
    c.clearRect(0, 0, w, h);
    const g = c.createLinearGradient(0, 0, w, h);
    g.addColorStop(0, "rgba(6,10,28,1)"); g.addColorStop(1, "rgba(3,4,12,1)");
    c.fillStyle = g; c.fillRect(0, 0, w, h);
    c.globalCompositeOperation = "lighter";
    for (let i = 0; i < 4; i++) {
      c.beginPath();
      const y0 = h * (0.25 + i * 0.16);
      c.moveTo(0, h);
      for (let x = 0; x <= w; x += 24) c.lineTo(x, y0 + Math.sin(x * 0.004 + t * 0.00035 * (1 + i * 0.4) + i) * h * 0.09 + Math.sin(x * 0.011 - t * 0.0005) * h * 0.02);
      c.lineTo(w, h); c.closePath();
      const [rr, gg, bb] = sdvigOttenka(rgb, i * 38 - 40);
      const f = c.createLinearGradient(0, y0 - h * 0.15, 0, y0 + h * 0.5);
      f.addColorStop(0, `rgba(${rr},${gg},${bb},0.30)`); f.addColorStop(1, `rgba(${rr},${gg},${bb},0)`);
      c.fillStyle = f; c.fill();
    }
    c.globalCompositeOperation = "source-over";
  };
}

const RISOVALKI: Partial<Record<VidFona, (k: Kontekst) => Ris>> = { planeta, galaktika, izgib, gradient };

function Holst({ vid }: { vid: VidFona }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const akcent = useUstanovki().akcent;
  useEffect(() => {
    const cv = ref.current;
    const g = cv?.getContext("2d");
    const sozdat = RISOVALKI[vid];
    if (!cv || !g || !sozdat) return;
    const rgb = (getComputedStyle(document.documentElement).getPropertyValue("--aurum-rgb").trim() || "231 194 125").split(/\s+/).map(Number) as [number, number, number];
    const k: Kontekst = { c: g, cv, w: 0, h: 0, rgb, mysh: { x: 0.5, y: 0.5 }, ochistka: [] };
    let dprMax = 1.5;
    const podgon = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, dprMax);
      k.w = cv.clientWidth; k.h = cv.clientHeight;
      cv.width = Math.round(k.w * dpr); cv.height = Math.round(k.h * dpr);
      g.setTransform(dpr, 0, 0, dpr, 0, 0);
    };
    podgon();
    const ro = new ResizeObserver(podgon); ro.observe(cv);
    const dvizh = (e: MouseEvent) => { k.mysh.x = e.clientX / window.innerWidth; k.mysh.y = e.clientY / window.innerHeight; };
    window.addEventListener("mousemove", dvizh);
    const ris = sozdat(k);
    const staticheski = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let id = 0, poslednee = performance.now(), medlenno = 0;
    const kadr = (t: number) => {
      const dt = Math.min(64, t - poslednee); poslednee = t;
      if (!document.hidden) {
        ris(t, dt);
        // не тянем 60 кадров: если кадр дольше 24 мс подряд, снижаем плотность пикселей
        medlenno = dt > 24 ? medlenno + 1 : Math.max(0, medlenno - 1);
        if (medlenno > 45 && dprMax > 1) { dprMax = 1; medlenno = 0; podgon(); }
      }
      if (!staticheski) id = requestAnimationFrame(kadr);
    };
    id = requestAnimationFrame(kadr);
    return () => { cancelAnimationFrame(id); ro.disconnect(); window.removeEventListener("mousemove", dvizh); k.ochistka.forEach((f) => f()); };
  }, [vid, akcent]);
  return <canvas ref={ref} className="aurum-holst" aria-hidden />;
}

// ---------- Переливы: шёлковый градиент на WebGL ----------
const PALITRY: Record<Palitra, string[]> = {
  okean: ["#040b26", "#0a3aa8", "#2f80ff", "#9bd6ff"],
  zakat: ["#14041f", "#7a1d72", "#f0507f", "#ffb870"],
  avrora: ["#02171a", "#0b6e66", "#3b52e0", "#86f7cf"],
  zoloto: ["#0b0804", "#6b4510", "#d19a38", "#ffe8ae"],
};
export const PALITRY_PREVIEW = PALITRY;

const VERSHINNYY = "attribute vec2 p;void main(){gl_Position=vec4(p,0.,1.);}";
const FRAGMENT = `precision highp float;
uniform vec2 uRes;uniform float uT;uniform vec2 uM;uniform vec3 c0,c1,c2,c3;
float hash(vec2 p){return fract(sin(dot(p,vec2(127.1,311.7)))*43758.5453);}
float noise(vec2 p){vec2 i=floor(p),f=fract(p);vec2 u=f*f*(3.-2.*f);
return mix(mix(hash(i),hash(i+vec2(1.,0.)),u.x),mix(hash(i+vec2(0.,1.)),hash(i+vec2(1.,1.)),u.x),u.y);}
float fbm(vec2 p){float v=0.,a=.5;for(int i=0;i<4;i++){v+=a*noise(p);p=p*2.02+vec2(1.7,9.2);a*=.5;}return v;}
void main(){
 vec2 uv=gl_FragCoord.xy/uRes;
 vec2 p=(gl_FragCoord.xy-.5*uRes)/min(uRes.x,uRes.y)*1.5+(uM-.5)*.18;
 float t=uT*.055;
 vec2 q=vec2(fbm(p+vec2(0.,t)),fbm(p+vec2(5.2,1.3)-t));
 vec2 r=vec2(fbm(p+2.2*q+vec2(1.7,9.2)+.8*t),fbm(p+2.2*q+vec2(8.3,2.8)-.6*t));
 float f=fbm(p+2.4*r);
 float f2=fbm(p+2.4*r+vec2(.018,.024));
 vec3 col=mix(c0,c1,smoothstep(.12,.75,f));
 col=mix(col,c2,smoothstep(.30,.95,length(q)*f*1.7));
 col=mix(col,c3,smoothstep(.55,1.,r.x*f*1.6)*.75);
 float sheen=clamp((f-f2)*30.,-1.,1.);
 col+=c3*max(sheen,0.)*.34;col*=1.-max(-sheen,0.)*.38;
 col*=.74+.46*f;
 float vg=smoothstep(1.3,.2,length(uv-.5)*1.35);col*=.5+.5*vg;
 gl_FragColor=vec4(col,1.);}`;

function hex3(h: string): [number, number, number] {
  const n = parseInt(h.slice(1), 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

function Perelivy({ palitra }: { palitra: Palitra }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const [net, setNet] = useState(false);
  useEffect(() => {
    const cv = ref.current;
    if (!cv) return;
    const gl = cv.getContext("webgl", { antialias: false, alpha: false, powerPreference: "low-power" });
    if (!gl) { setNet(true); return; }
    const shader = (tip: number, src: string) => { const s = gl.createShader(tip)!; gl.shaderSource(s, src); gl.compileShader(s); return gl.getShaderParameter(s, gl.COMPILE_STATUS) ? s : null; };
    const vs = shader(gl.VERTEX_SHADER, VERSHINNYY), fs = shader(gl.FRAGMENT_SHADER, FRAGMENT);
    const prog = gl.createProgram();
    if (!vs || !fs || !prog) { setNet(true); return; }
    gl.attachShader(prog, vs); gl.attachShader(prog, fs); gl.linkProgram(prog);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) { setNet(true); return; }
    gl.useProgram(prog);
    const buf = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    const loc = gl.getAttribLocation(prog, "p"); gl.enableVertexAttribArray(loc); gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0);
    const u = (n: string) => gl.getUniformLocation(prog, n);
    const [uRes, uT, uM] = [u("uRes"), u("uT"), u("uM")];
    PALITRY[palitra].forEach((hx, i) => gl.uniform3fv(u(`c${i}`), hex3(hx)));
    let shkala = 0.5;
    const podgon = () => { cv.width = Math.max(2, Math.round(cv.clientWidth * shkala)); cv.height = Math.max(2, Math.round(cv.clientHeight * shkala)); gl.viewport(0, 0, cv.width, cv.height); };
    podgon();
    const ro = new ResizeObserver(podgon); ro.observe(cv);
    const m = { x: 0.5, y: 0.5, tx: 0.5, ty: 0.5 };
    const dv = (e: MouseEvent) => { m.tx = e.clientX / window.innerWidth; m.ty = 1 - e.clientY / window.innerHeight; };
    window.addEventListener("mousemove", dv);
    const poteryan = (e: Event) => { e.preventDefault(); setNet(true); };
    cv.addEventListener("webglcontextlost", poteryan);
    const staticheski = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let id = 0, t = 0, pr = performance.now(), medlenno = 0;
    const kadr = (now: number) => {
      const dt = Math.min(64, now - pr); pr = now;
      if (!document.hidden) {
        t += dt * 0.001;
        m.x += (m.tx - m.x) * 0.04; m.y += (m.ty - m.y) * 0.04;
        gl.uniform2f(uRes, cv.width, cv.height); gl.uniform1f(uT, t); gl.uniform2f(uM, m.x, m.y);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
        medlenno = dt > 24 ? medlenno + 1 : Math.max(0, medlenno - 1);
        if (medlenno > 45 && shkala > 0.3) { shkala = 0.33; medlenno = 0; podgon(); }
      }
      if (!staticheski) id = requestAnimationFrame(kadr);
    };
    id = requestAnimationFrame(kadr);
    return () => { cancelAnimationFrame(id); ro.disconnect(); window.removeEventListener("mousemove", dv); cv.removeEventListener("webglcontextlost", poteryan); };
  }, [palitra]);
  if (net) return <Holst vid="izgib" />;
  return <canvas ref={ref} className="aurum-holst aurum-perelivy" aria-hidden />;
}

export function Fon() {
  const u = useUstanovki();
  const svoy = u.fon === "svoy";
  return (
    <div className="aurum-fon" data-vid={u.fon} aria-hidden>
      <div className="aurum-fon-baza" />
      {u.fon === "perelivy" && <Perelivy palitra={u.palitra} />}
      {svoy && u.mediaUrl && u.tipMedia === "video" && <video className="aurum-fon-video" src={u.mediaUrl} autoPlay loop muted playsInline />}
      {svoy && u.mediaUrl && u.tipMedia === "image" && <div className="aurum-fon-kartinka" style={{ backgroundImage: `url("${u.mediaUrl}")` }} />}
      {svoy && !u.mediaUrl && <div className="aurum-fon-pusto" />}
      {(u.fon === "planeta" || u.fon === "galaktika" || u.fon === "izgib" || u.fon === "gradient") && <Holst vid={u.fon} />}
      <div className="aurum-fon-vinetka" />
      {u.zerno && <div className="aurum-zerno" />}
    </div>
  );
}
