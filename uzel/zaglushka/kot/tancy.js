// Танцы котов: каждая манера это поза как функция доли трека. Общий модуль,
// страницы берут его по /lib/nashe/tancy.js.
//
// TANCY[imya](p, o): p номер доли с дробью (целое значит удар), o.sila от 0
// до 1 (размах: в куплете тише, в дропе во всю силу), o.faza сдвиг в долях
// для плавных движений, чтобы толпа не качалась как один кот. o.polnaya
// включает большие движения (оборот, прыжок на каждую долю); по умолчанию
// это sila от 0.75. Если сила растёт плавно, полноту лучше менять только на
// начале такта, иначе прыжок или оборот сорвётся на середине. Удары у всех
// приходятся точно на долю. Возвращает позу для cat.update; рот, моргание
// и взгляд страница дописывает сама.
//
// Смысл каналов позы (REST в kot.js): arm_lift 0.45 рука опущена, 2.3 над
// головой; arm_fwd больше нуля вперёд, меньше назад; arm_elbow сгиб локтя;
// arm_elbowZ меньше нуля предплечье к телу. pelvisY 0.49 стоит, меньше
// значит присел. yaw поворот всего кота.
import { REST } from './kot.js';

export const clamp = (x, a = 0, b = 1) => Math.min(b, Math.max(a, x));
export const lerp = (a, b, k) => a + (b - a) * k;
export const ss = (a, b, x) => { const k = clamp((x - a) / (b - a)); return k * k * (3 - 2 * k); };
export const frac = (x) => x - Math.floor(x);
export const bump = (x, c, w) => Math.exp(-(((x - c) / w) ** 2));
export const sideOf = (p) => ((((Math.floor(p) % 2) + 2) % 2) === 0 ? -1 : 1);

// Удар на долю, 1 в момент доли: подготовка, быстрый спуск в долю, отдача
// с перелётом, успокоение
export function pulse(p) {
  const f = frac(p);
  const after = Math.exp(-f * 5.5) * Math.cos(f * Math.PI * 1.6);
  const before = ss(0.8, 1.0, f) ** 2;
  const antic = -0.2 * bump(f, 0.68, 0.08);
  return Math.max(after, before) + antic;
}

// Перекачка: на чётной доле -1, на нечётной +1, переход ближе к доле
export function alt(p) {
  const f = frac(p), from = sideOf(p);
  const over = Math.exp(-f * 6) * Math.sin(f * Math.PI * 2) * 0.25;
  return lerp(from, -from, ss(0.3, 1.0, f)) + from * over;
}

export function mixPose(A, B, k) {
  if (k <= 0) return A;
  if (k >= 1) return B;
  const P = {};
  for (const key in A) P[key] = lerp(A[key], B[key], k);
  return P;
}

// Ноги в прыжке поджаты, носки вниз; u от 0 (толчок) до 1 (приземление)
function nogiVPryzhke(P, u) {
  const y = Math.max(0.09, P.pelvisY - 0.32 + 0.06 * bump(u, 0.5, 0.25));
  P.footL_y = P.footR_y = y;
  P.footL_pitch = P.footR_pitch = 0.6 * ss(0.05, 0.3, u) * (1 - ss(0.75, 0.95, u));
}

// Общая основа: пружина в коленях на долю, кивок, уши и хвост следом
function osnova(p, k) {
  const P = { ...REST };
  const pu = pulse(p);
  P.pelvisY = 0.49 - 0.045 * k * pu;
  P.squash = 1 - 0.05 * k * pu;
  P.headNod = 0.13 * k * pulse(p - 0.15);
  P.earFlop = k * pulse(p - 0.3);
  P.tailPhase = Math.PI * (p - 0.25);
  P.tailAmp = 0.25 + 0.3 * k;
  P.smile = 0.9;
  return P;
}

// Руки над головой буквой V: строго вверх их прячет широкая голова
function rukiVverh(P, a, k, razmah = 0.3) {
  const h = lerp(1.2, 2.35, k);
  P.armL_lift = h - razmah * a;
  P.armR_lift = h + razmah * a;
  P.armL_fwd = P.armR_fwd = 0.25;
  P.armL_elbow = P.armR_elbow = lerp(0.6, 0.15, k);
}

export const TANCY = {
  // Качается всем телом в полтемпа, руки свободно болтаются
  kach(p, { sila: k = 1, faza = 0 } = {}) {
    const P = osnova(p, k);
    const a = alt((p + faza) / 2);
    P.lean = 0.22 * k * a;
    P.hipSway = -0.1 * k * a;
    P.pelvisX = 0.09 * k * a;
    P.headTilt = -0.2 * k * a;
    P.chestSide = 0.08 * k * a;
    P.armL_lift = 0.6 + 0.6 * k * Math.max(0, -a);
    P.armR_lift = 0.6 + 0.6 * k * Math.max(0, a);
    P.armL_fwd = P.armR_fwd = 0.3;
    P.armL_elbow = P.armR_elbow = 0.5;
    P.footL_x = 0.22;
    P.footR_x = -0.22;
    P.squint = 0.3;
    return P;
  },

  // Машет поднятыми лапами с палочками, локти гуляют в такт. Лапы вынесены
  // вперёд и в стороны: над головой их и палочки не видно из-за головы
  mashet(p, { sila: k = 1 } = {}) {
    const P = osnova(p, k);
    const a = alt(p);
    P.armL_lift = lerp(1.1, 1.85, k) - 0.35 * a;
    P.armR_lift = lerp(1.1, 1.85, k) + 0.35 * a;
    P.armL_fwd = P.armR_fwd = 0.6;
    P.armL_elbow = P.armR_elbow = 0.35;
    P.armL_elbowZ = 0.5 * alt(p - 0.2);
    P.armR_elbowZ = -0.5 * alt(p - 0.2);
    P.lean = 0.06 * k * a;
    P.headTilt = 0.1 * alt(p - 0.15);
    P.footL_x = 0.2;
    P.footR_x = -0.2;
    P.footL_y = 0.09 + 0.05 * k * Math.max(0, -a);
    P.footR_y = 0.09 + 0.05 * k * Math.max(0, a);
    P.mouth = 0.35 * k * Math.max(0, pulse(p));
    P.smile = 1;
    return P;
  },

  // Шаг с переносом веса, а каждые 8 долей полный оборот (с силы 0.75)
  kruzhitsya(p, { sila: k = 1, polnaya = k >= 0.75 } = {}) {
    const P = osnova(p, k);
    const a = alt(p);
    const u = ((p % 8) + 8) % 8;
    const krug = polnaya ? ss(5.9, 7.7, u) : 0;
    P.yaw = 2 * Math.PI * krug;
    P.pelvisX = 0.08 * k * a * (1 - krug);
    P.hipSway = 0.12 * k * a;
    P.headTilt = -0.12 * alt(p - 0.18);
    const vKruge = polnaya ? bump(u, 6.8, 0.9) : 0;
    P.armL_lift = lerp(0.7 + 0.5 * Math.max(0, a), 1.5, vKruge);
    P.armR_lift = lerp(0.7 + 0.5 * Math.max(0, -a), 1.5, vKruge);
    P.armL_fwd = P.armR_fwd = lerp(0.6, 0.2, vKruge);
    P.armL_elbow = P.armR_elbow = lerp(1.2, 0.3, vKruge);
    P.footL_x = 0.22;
    P.footR_x = -0.22;
    P.footR_y = 0.09 + 0.06 * k * Math.max(0, a);
    P.footL_y = 0.09 + 0.06 * k * Math.max(0, -a);
    P.happy = ss(0.3, 0.6, vKruge);
    P.mouth = 0.4 * vKruge;
    P.smile = 1;
    P.tailAmp = 0.5;
    return P;
  },

  // Прыгает на месте: в полную силу на каждую долю, тише через долю
  prygaet(p, { sila: k = 1, polnaya = k >= 0.75 } = {}) {
    const P = osnova(p, 0);
    const shag = polnaya ? 1 : 2;
    const u = frac(p / shag);
    const h = lerp(0.07, 0.15, k) * 4 * u * (1 - u);
    const posadka = bump(u, 0, 0.07) + bump(u, 1, 0.07);
    P.pelvisY = 0.49 - 0.07 * posadka + h;
    P.squash = 1 - 0.1 * posadka + 0.05 * bump(u, 0.15, 0.1);
    const ruki = Math.sin(Math.PI * u);
    P.armL_lift = P.armR_lift = lerp(0.6, lerp(1.4, 2.3, k), ruki);
    P.armL_fwd = P.armR_fwd = 0.35;
    P.armL_elbow = P.armR_elbow = lerp(1.0, 0.3, ruki);
    P.footL_x = 0.16;
    P.footR_x = -0.16;
    nogiVPryzhke(P, u);
    P.happy = ss(0.3, 0.45, u) * (1 - ss(0.7, 0.85, u));
    P.mouth = 0.5 * ruki;
    P.smile = 1;
    P.earFlop = posadka;
    P.tailAmp = 0.45;
    P.tailLift = 0.4 * ruki;
    return P;
  },

  // Руки за спиной, солидно кивает на каждую долю, сильнее на первую в такте
  kivaet(p, { sila: k = 1, faza = 0 } = {}) {
    const P = osnova(p, k * 0.6);
    const pervaya = (((Math.round(p) % 4) + 4) % 4) === 0 ? 1.4 : 1;
    P.headNod = 0.26 * k * pervaya * pulse(p - 0.08) - 0.05;
    P.chestBend = 0.06 * k * pulse(p - 0.05);
    P.armL_lift = P.armR_lift = 0.3;
    P.armL_fwd = P.armR_fwd = -0.55;
    P.armL_elbow = P.armR_elbow = 0.8;
    P.headTilt = 0.08 * Math.sin(Math.PI * (p + faza) / 4);
    P.lookY = 0.2;
    P.squint = 0.35;
    P.smile = 0.75;
    P.footL_x = 0.17;
    P.footR_x = -0.17;
    return P;
  },

  // Трясёт головой как на рок-концерте, правая лапа бьёт вверх
  tryaset(p, { sila: k = 1 } = {}) {
    const P = osnova(p, k);
    const pu = pulse(p);
    P.headNod = 0.38 * k * pu - 0.12;
    P.chestBend = 0.12 * k * pulse(p - 0.05) + 0.05;
    P.headTilt = 0.12 * k * Math.sin(Math.PI * 2 * p);
    P.armR_lift = lerp(0.9, 2.2, k * (0.6 + 0.4 * Math.max(0, pu)));
    P.armR_fwd = 0.4;
    P.armR_elbow = 0.35 + 0.5 * (1 - Math.max(0, pu));
    P.armL_lift = 0.65;
    P.armL_fwd = 0.5;
    P.armL_elbow = 1.3;
    P.pelvisY -= 0.04 * k;
    P.footL_x = 0.24;
    P.footR_x = -0.24;
    P.squint = 0.5;
    P.lidTilt = -0.5;
    P.mouth = 0.45 * k * Math.max(0, pu);
    P.earBack = 0.4 * k;
    return P;
  },

  // Плавные волны руками над головой, тело тянется следом
  volna(p, { sila: k = 1, faza = 0 } = {}) {
    const P = osnova(p, k * 0.7);
    const s = Math.sin(Math.PI * (p + faza) / 2);
    const s2 = Math.sin(Math.PI * (p + faza) / 2 - 0.9);
    rukiVverh(P, s, k, 0.35);
    P.armL_elbowZ = 0.5 * s2;
    P.armR_elbowZ = -0.5 * s2;
    P.lean = 0.12 * k * s;
    P.hipSway = -0.08 * k * s;
    P.headTilt = 0.12 * s2;
    P.squint = 0.25;
    P.smile = 1;
    P.footL_x = 0.18;
    P.footR_x = -0.18;
    return P;
  },

  // Двигает плечами: грудь качается вбок, локти согнуты, лапы вверх по очереди
  plechi(p, { sila: k = 1 } = {}) {
    const P = osnova(p, k);
    const a = alt(p);
    P.chestSide = 0.14 * k * a;
    P.twist = 0.18 * k * alt(p - 0.1);
    P.headTilt = -0.12 * k * a;
    P.armL_lift = 0.7 + 0.45 * k * Math.max(0, a);
    P.armR_lift = 0.7 + 0.45 * k * Math.max(0, -a);
    P.armL_fwd = P.armR_fwd = 0.55;
    P.armL_elbow = P.armR_elbow = 1.5;
    P.footL_x = 0.22;
    P.footR_x = -0.22;
    P.squint = 0.4;
    P.lookX = 0.4 * a;
    P.smile = 1;
    return P;
  },

  // Приставной шаг вбок и обратно, лапы качаются навстречу шагу
  shag(p, { sila: k = 1 } = {}) {
    const P = osnova(p, k);
    const a = alt(p / 2);
    const nogi = alt(p);
    P.x = 0.14 * k * a;
    P.pelvisX = 0.04 * k * nogi;
    P.hipSway = 0.1 * k * nogi;
    P.footL_x = 0.2 + 0.06 * k * Math.max(0, nogi);
    P.footR_x = -0.2 - 0.06 * k * Math.max(0, -nogi);
    P.footR_y = 0.09 + 0.07 * k * Math.max(0, nogi);
    P.footL_y = 0.09 + 0.07 * k * Math.max(0, -nogi);
    P.armL_fwd = 0.2 + 0.45 * k * nogi;
    P.armR_fwd = 0.2 - 0.45 * k * nogi;
    P.armL_lift = P.armR_lift = 0.6;
    P.armL_elbow = P.armR_elbow = 0.9;
    P.yaw = 0.15 * k * a;
    P.headTilt = -0.1 * nogi;
    P.lookX = 0.5 * a;
    P.smile = 1;
    return P;
  },

  // Хлопает перед грудью на вторую и четвёртую долю
  hlopaet(p, { sila: k = 1 } = {}) {
    const P = osnova(p, k);
    // Хлопок на нечётных долях: ближайшая нечётная доля и расстояние до неё
    const q = (p - 1) / 2;
    const d = (q - Math.round(q)) * 2;
    const vmeste = Math.exp(-(d * d) / 0.03);
    P.armL_lift = P.armR_lift = lerp(0.85, 0.12, vmeste);
    P.armL_fwd = P.armR_fwd = lerp(1.15, lerp(1.25, 1.75, k), vmeste);
    P.armL_elbow = P.armR_elbow = lerp(1.0, 0.45, vmeste);
    P.headNod += 0.08 * vmeste;
    P.happy = 0.8 * vmeste;
    P.mouth = 0.3 * vmeste;
    P.smile = 1;
    P.footL_x = 0.18;
    P.footR_x = -0.18;
    return P;
  },

  // Важно покачивается: левая лапа на бедре, правая жестикулирует на первую долю
  vazhno(p, { sila: k = 1, faza = 0 } = {}) {
    const P = osnova(p, k * 0.5);
    const a = alt((p + faza) / 2);
    P.lean = 0.07 * k * a;
    P.pitch = -0.04;
    P.headNod = -0.1 + 0.12 * k * pulse(p / 4);
    P.armL_lift = 0.75;
    P.armL_fwd = -0.25;
    P.armL_elbow = 0.2;
    P.armL_elbowZ = -1.9;
    const zhest = pulse(p / 4);
    P.armR_lift = 0.6 + 0.6 * k * Math.max(0, zhest);
    P.armR_fwd = 0.6;
    P.armR_elbow = 1.1 - 0.5 * Math.max(0, zhest);
    P.lookY = 0.25;
    P.squint = 0.45;
    P.smile = 0.7;
    P.footL_x = 0.15;
    P.footR_x = -0.15;
    return P;
  },

  // Глубоко пружинит в коленях, кулачки качают вперёд по очереди
  pruzhinit(p, { sila: k = 1 } = {}) {
    const P = osnova(p, 0);
    const pu = pulse(p);
    const a = alt(p);
    P.pelvisY = 0.49 - 0.11 * k * Math.max(0, pu);
    P.squash = 1 - 0.07 * k * Math.max(0, pu);
    P.chestBend = 0.08 * k * pu;
    P.armL_fwd = 0.4 + 0.7 * k * Math.max(0, a);
    P.armR_fwd = 0.4 + 0.7 * k * Math.max(0, -a);
    P.armL_lift = P.armR_lift = 0.6;
    P.armL_elbow = P.armR_elbow = 1.3;
    P.footL_x = 0.24;
    P.footR_x = -0.24;
    P.headNod = 0.15 * k * pulse(p - 0.12);
    P.mouth = 0.3 * k * Math.max(0, pu);
    P.smile = 1;
    return P;
  },

  // Именинница: перекачка с ударом лап вверх, на каждый второй такт
  // поворот на месте и лапы над головой
  imeninnica(p, { sila: k = 1, polnaya = k >= 0.75 } = {}) {
    const P = osnova(p, k);
    const a = alt(p), wL = (a + 1) / 2;
    const u = ((p % 8) + 8) % 8;
    const pokaz = polnaya ? bump(u, 6.6, 1.0) : 0;
    P.yaw = 0.35 * Math.sin((2 * Math.PI * p) / 8) * (1 - pokaz);
    P.pelvisX = 0.08 * k * a;
    P.hipSway = 0.12 * k * a;
    P.headTilt = -0.12 * alt(p - 0.18);
    const vverh = { lift: lerp(1.5, 2.3, k), fwd: 0.35, elbow: 0.25 }, vniz = { lift: 0.6, fwd: 0.7, elbow: 1.4 };
    P.armL_lift = lerp(lerp(vniz.lift, vverh.lift, wL), 2.2, pokaz);
    P.armR_lift = lerp(lerp(vverh.lift, vniz.lift, wL), 2.2, pokaz);
    P.armL_fwd = lerp(lerp(vniz.fwd, vverh.fwd, wL), 0.3, pokaz);
    P.armR_fwd = lerp(lerp(vverh.fwd, vniz.fwd, wL), 0.3, pokaz);
    P.armL_elbow = lerp(lerp(vniz.elbow, vverh.elbow, wL), 0.15, pokaz);
    P.armR_elbow = lerp(lerp(vverh.elbow, vniz.elbow, wL), 0.15, pokaz);
    P.footL_x = 0.22;
    P.footR_x = -0.22;
    P.footR_y = 0.09 + 0.06 * k * Math.max(0, a);
    P.footL_y = 0.09 + 0.06 * k * Math.max(0, -a);
    P.happy = ss(0.4, 0.7, pokaz);
    P.mouth = 0.25 * Math.max(0, pulse(p)) + 0.4 * pokaz;
    P.smile = 1;
    P.tailAmp = 0.55;
    P.tailLift = 0.3 * pokaz;
    return P;
  },
};

// Диджей за пультом. o.skretch от -1 до 1: правая лапа водит пластинку
// вперёд и назад; o.ruka от 0 до 1: левая лапа уходит с пульта вверх
// (на дроп); o.sila размах кивков и пружины.
export function didzhey(p, { sila: k = 1, skretch = 0, ruka = 0 } = {}) {
  const P = osnova(p, k);
  P.headNod = 0.2 * k * pulse(p - 0.1) + 0.12;
  P.chestBend = 0.12 + 0.04 * k * pulse(p - 0.05);
  P.headTilt = 0.08 * alt(p / 2);
  // Обе лапы на пульте перед грудью
  P.armR_lift = 0.4;
  P.armR_fwd = 1.05 + 0.25 * skretch;
  P.armR_elbow = 0.75 - 0.3 * skretch;
  P.armL_lift = lerp(0.4, 2.3, ruka);
  P.armL_fwd = lerp(1.05, 0.3, ruka);
  P.armL_elbow = lerp(0.75, 0.15 + 0.3 * Math.max(0, pulse(p)), ruka);
  P.footL_x = 0.2;
  P.footR_x = -0.2;
  P.squint = 0.3;
  P.lookY = -0.35;
  P.smile = 0.95;
  return P;
}
