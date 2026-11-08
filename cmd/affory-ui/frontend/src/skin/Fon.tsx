// Живые фоны AURUM. Всё рисуется на canvas без единого файла и без сети:
// планета из точек, спиральная галактика, цветные изгибы, северное сияние и
// своя картинка. Фон лежит под #root и событий не ловит.
import { useEffect, useRef } from "react";
import { useUstanovki, type Fon as VidFona } from "./ustanovki";

type Ris = (t: number, dt: number) => void;
interface Kontekst { c: CanvasRenderingContext2D; w: number; h: number; rgb: [number, number, number]; mysh: { x: number; y: number } }

const TAU = Math.PI * 2;
const rgba = (rgb: number[], a: number) => `rgba(${rgb[0]},${rgb[1]},${rgb[2]},${a})`;

// ---------- Планета ----------
// Грубые контуры материков в градусах (долгота, широта). Нужны не для
// географии, а чтобы точечная сфера читалась как Земля, как на референсе.
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

function planeta(k: Kontekst): Ris {
  const m = maska();
  const N = 4200;
  const tochki: { x: number; y: number; z: number; sushi: boolean }[] = [];
  const zol = Math.PI * (3 - Math.sqrt(5));
  for (let i = 0; i < N; i++) {
    const y = 1 - (i / (N - 1)) * 2;
    const r = Math.sqrt(1 - y * y);
    const th = zol * i;
    const x = Math.cos(th) * r, z = Math.sin(th) * r;
    const lat = Math.asin(y) * (180 / Math.PI);
    const lon = Math.atan2(x, z) * (180 / Math.PI);
    const px = Math.min(MW - 1, Math.max(0, Math.floor(lon + 180)));
    const py = Math.min(MH - 1, Math.max(0, Math.floor(90 - lat)));
    tochki.push({ x, y, z, sushi: m[(py * MW + px) * 4 + 3] > 100 });
  }
  let ugol = 0.9;
  return (_t, dt) => {
    const { c, w, h, rgb } = k;
    c.clearRect(0, 0, w, h);
    const R = Math.min(w, h) * 0.36;
    const cx = w * 0.5, cy = h * 0.5;
    ugol += dt * 0.00007;
    const tilt = 0.38 + (k.mysh.y - 0.5) * 0.18;
    const ca = Math.cos(ugol + (k.mysh.x - 0.5) * 0.3), sa = Math.sin(ugol + (k.mysh.x - 0.5) * 0.3);
    const ct = Math.cos(tilt), st = Math.sin(tilt);
    // ореол
    const g = c.createRadialGradient(cx, cy, R * 0.6, cx, cy, R * 1.35);
    g.addColorStop(0, rgba(rgb, 0.0)); g.addColorStop(0.72, rgba(rgb, 0.10)); g.addColorStop(1, rgba(rgb, 0));
    c.fillStyle = g; c.fillRect(0, 0, w, h);
    // тело сферы: стекло
    const b = c.createRadialGradient(cx - R * 0.35, cy - R * 0.4, R * 0.1, cx, cy, R);
    b.addColorStop(0, "rgba(255,255,255,0.07)"); b.addColorStop(1, "rgba(255,255,255,0.015)");
    c.beginPath(); c.arc(cx, cy, R, 0, TAU); c.fillStyle = b; c.fill();
    c.lineWidth = 1.2; c.strokeStyle = "rgba(255,255,255,0.16)"; c.stroke();
    for (const p of tochki) {
      const x1 = p.x * ca + p.z * sa, z1 = -p.x * sa + p.z * ca;
      const y2 = p.y * ct - z1 * st, z2 = p.y * st + z1 * ct;
      if (z2 < -0.02 && !p.sushi) continue;
      const lic = z2 > 0;
      const a = p.sushi ? (lic ? 0.35 + 0.65 * z2 : 0.07) : (lic ? 0.06 + 0.1 * z2 : 0);
      if (a <= 0.01) continue;
      const r = p.sushi ? (lic ? 1.1 + 1.0 * z2 : 0.7) : 0.7;
      c.fillStyle = p.sushi && lic ? `rgba(255,252,244,${a})` : `rgba(210,220,235,${a})`;
      c.beginPath(); c.arc(cx + x1 * R, cy - y2 * R, r, 0, TAU); c.fill();
    }
    // тонкое кольцо-орбита
    c.save(); c.translate(cx, cy); c.rotate(-0.38); c.scale(1, 0.26);
    c.beginPath(); c.arc(0, 0, R * 1.2, 0, TAU);
    c.lineWidth = 1; c.strokeStyle = rgba(rgb, 0.28); c.stroke(); c.restore();
  };
}

// ---------- Галактика ----------
function galaktika(k: Kontekst): Ris {
  const N = 6200;
  const z = Array.from({ length: N }, (_, i) => {
    const ruk = i % 3;
    const r = Math.pow(Math.random(), 0.62);
    const ugl = ruk * (TAU / 3) + r * 7.2 + (Math.random() - 0.5) * (0.32 + (1 - r) * 0.4);
    return { r, ugl, s: 0.6 + Math.random() * 1.7, f: Math.random() * TAU, t: Math.random() };
  });
  const fon = Array.from({ length: 140 }, () => ({ x: Math.random(), y: Math.random(), s: Math.random() * 1.2 + 0.2, f: Math.random() * TAU }));
  let phi = 0;
  return (t, dt) => {
    const { c, w, h, rgb } = k;
    c.clearRect(0, 0, w, h);
    phi += dt * 0.00004;
    for (const s of fon) {
      c.fillStyle = `rgba(255,255,255,${0.15 + 0.25 * (0.5 + 0.5 * Math.sin(t * 0.0006 + s.f))})`;
      c.fillRect(s.x * w, s.y * h, s.s, s.s);
    }
    const cx = w * (0.5 + (k.mysh.x - 0.5) * 0.04), cy = h * (0.5 + (k.mysh.y - 0.5) * 0.04);
    const R = Math.min(w * 0.62, h * 0.95);
    const nakl = 0.52;
    const yadro = c.createRadialGradient(cx, cy, 0, cx, cy, R * 0.34);
    yadro.addColorStop(0, "rgba(255,244,222,0.55)"); yadro.addColorStop(0.35, rgba(rgb, 0.22)); yadro.addColorStop(1, rgba(rgb, 0));
    c.fillStyle = yadro; c.fillRect(cx - R, cy - R, R * 2, R * 2);
    c.globalCompositeOperation = "lighter";
    for (const s of z) {
      const a = s.ugl + phi * (1.6 - s.r);
      const x = Math.cos(a) * s.r * R;
      const y = Math.sin(a) * s.r * R * nakl;
      const tw = 0.55 + 0.45 * Math.sin(t * 0.002 + s.f);
      const yar = (0.38 + (1 - s.r) * 0.6) * tw;
      // ближе к центру теплее, к краю цвет акцента, у дальних рук холодный
      const ton = s.r < 0.25 ? "255,236,200" : s.t > 0.8 ? "170,200,255" : `${rgb[0]},${rgb[1]},${rgb[2]}`;
      c.fillStyle = `rgba(${ton},${Math.min(0.9, yar)})`;
      c.fillRect(cx + x, cy + y, s.s, s.s);
    }
    c.globalCompositeOperation = "source-over";
  };
}

// ---------- Цветные изгибы ----------
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

function sdvigOttenka([r, g, b]: number[], grad: number): [number, number, number] {
  // поворот в YIQ: дёшево и достаточно для фона
  const a = (grad * Math.PI) / 180, cs = Math.cos(a), sn = Math.sin(a);
  const Y = 0.299 * r + 0.587 * g + 0.114 * b, I = 0.596 * r - 0.274 * g - 0.322 * b, Q = 0.211 * r - 0.523 * g + 0.312 * b;
  const I2 = I * cs - Q * sn, Q2 = I * sn + Q * cs;
  const cl = (v: number) => Math.max(0, Math.min(255, Math.round(v)));
  return [cl(Y + 0.956 * I2 + 0.621 * Q2), cl(Y - 0.272 * I2 - 0.647 * Q2), cl(Y - 1.106 * I2 + 1.703 * Q2)];
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
      for (let x = 0; x <= w; x += 24) {
        const y = y0 + Math.sin(x * 0.004 + t * 0.00035 * (1 + i * 0.4) + i) * h * 0.09 + Math.sin(x * 0.011 - t * 0.0005) * h * 0.02;
        c.lineTo(x, y);
      }
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
    const rgbStr = getComputedStyle(document.documentElement).getPropertyValue("--aurum-rgb").trim() || "231 194 125";
    const rgb = rgbStr.split(/\s+/).map(Number) as [number, number, number];
    const k: Kontekst = { c: g, w: 0, h: 0, rgb, mysh: { x: 0.5, y: 0.5 } };
    const podgon = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
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
    let id = 0, poslednee = performance.now();
    const kadr = (t: number) => {
      const dt = Math.min(64, t - poslednee); poslednee = t;
      if (!document.hidden) ris(t, dt);
      if (!staticheski) id = requestAnimationFrame(kadr);
    };
    id = requestAnimationFrame(kadr);
    return () => { cancelAnimationFrame(id); ro.disconnect(); window.removeEventListener("mousemove", dvizh); };
  }, [vid, akcent]);
  return <canvas ref={ref} className="aurum-holst" aria-hidden />;
}

export function Fon() {
  const u = useUstanovki();
  return (
    <div className="aurum-fon" data-vid={u.fon} aria-hidden>
      <div className="aurum-fon-baza" />
      {u.fon === "svoy" && u.kartinka && <div className="aurum-fon-kartinka" style={{ backgroundImage: `url("${u.kartinka}")` }} />}
      {u.fon === "svoy" && !u.kartinka && <div className="aurum-fon-pusto" />}
      {(u.fon === "planeta" || u.fon === "galaktika" || u.fon === "izgib" || u.fon === "gradient") && <Holst vid={u.fon} />}
      <div className="aurum-fon-vinetka" />
      {u.zerno && <div className="aurum-zerno" />}
    </div>
  );
}
