// Мультяшный кот: одна гладкая поверхность на скелете. Общий модуль, страницы
// роликов берут его по /lib/nashe/kot.js.
//
// Форма задана расстояниями до простых тел (эллипсоиды, капсулы, конусы),
// слитых плавно, как глина. Из неё строится сетка (surface nets: вершина в
// каждой клетке решётки, где поверхность её пересекает), каждая вершина
// привязывается к костям по близости к их телам. Сетка строится один раз на
// страницу и делится между всеми котами. Окрас считается в шейдере по точке
// тела в покое, поэтому полоски едут вместе с телом.
//
// Свой у каждого кота: окрас (OKRAS), сложение (рост, голова, толщина) и
// наряд, который вешается на кости через prikrepit (см. naryad.js).
//
// Поза задаётся плоским объектом каналов (REST), кот её только применяет:
// хореография живёт снаружи. Кот смотрит на +z, его левая сторона +x.
// Ноги ставятся решением двух звеньев: стопы стоят там, куда их поставили.
import * as THREE from 'three';

export const REST = {
  x: 0, yaw: 0, pelvisX: 0, pelvisY: 0.49, pitch: 0, lean: 0, hipSway: 0, squash: 1,
  chestBend: 0, chestSide: 0, twist: 0,
  headNod: 0, headTilt: 0, headTurn: 0, earBack: 0, earFlop: 0,
  armL_lift: 0.45, armL_fwd: 0.1, armL_elbow: 0.3, armL_elbowZ: 0,
  armR_lift: 0.45, armR_fwd: 0.1, armR_elbow: 0.3, armR_elbowZ: 0,
  footL_x: 0.15, footL_y: 0.09, footL_z: 0.05, footL_pitch: 0,
  footR_x: -0.15, footR_y: 0.09, footR_z: 0.05, footR_pitch: 0,
  tailAmp: 0.3, tailLift: 0, tailSide: 0, tailPhase: 0,
  // Лицо: blink закрывает верхнее веко, squint поднимает нижнее, happy
  // зажмуривает дугами, lidTilt наклоняет веки (-1 решительно, +1 жалобно),
  // smile поднимает уголки рта, mouth открывает рот
  blinkL: 0, blinkR: 0, squint: 0, happy: 0, lidTilt: 0, smile: 0.7, mouth: 0, lookX: 0, lookY: 0,
};

// Окрас по умолчанию: рыжий полосатый. Цвета в sRGB, в шейдер уходят линейными.
// polosyVes, pyatnaVes, svetlyyVes: сила полосок, пятен и светлых мест 0..1.
// Пятна (как у трёхцветной кошки) из крупного шума, sid сдвигает их у каждого кота.
// В шейдере только ASCII: patch и подобные слова там зарезервированы
// Пятен два слоя: у трёхцветной кошки рыжие и чёрные. blesk: цвет бархатного
// отлива шерсти, у тёмных котов его надо брать холоднее
export const OKRAS = {
  osnova: '#f7922f', svetlyy: '#ffe2bd', polosy: '#d0601d', rozovyy: '#ffa0af', pyatno: '#2a2522',
  pyatno2: '#2a2522', glaza: '#6cc23e', blesk: '#ffd2a0',
  polosyVes: 1, pyatnaVes: 0, pyatna2Ves: 0, svetlyyVes: 1, sid: 0,
};
const CAT_COLOR_GLSL = /* glsl */ `
uniform vec3 uOsnova; uniform vec3 uSvetlyy; uniform vec3 uPolosy; uniform vec3 uRozovyy; uniform vec3 uPyatno;
uniform vec3 uPyatno2; uniform float uPyatna2Ves;
uniform float uPolosyVes; uniform float uPyatnaVes; uniform float uSvetlyyVes; uniform float uSid;
float catH3(vec3 p) { p = fract(p * 0.3183099 + 0.1); p *= 17.0; return fract(p.x * p.y * p.z * (p.x + p.y + p.z)); }
float catNoise(vec3 x) {
  vec3 i = floor(x), f = fract(x);
  f = f * f * (3.0 - 2.0 * f);
  return mix(mix(mix(catH3(i), catH3(i + vec3(1, 0, 0)), f.x), mix(catH3(i + vec3(0, 1, 0)), catH3(i + vec3(1, 1, 0)), f.x), f.y),
             mix(mix(catH3(i + vec3(0, 0, 1)), catH3(i + vec3(1, 0, 1)), f.x), mix(catH3(i + vec3(0, 1, 1)), catH3(i + vec3(1, 1, 1)), f.x), f.y), f.z);
}
float catIn(vec3 p, vec3 c, vec3 r, float soft) { return 1.0 - smoothstep(1.0 - soft, 1.0, length((p - c) / r)); }
float catBand(float v, float c, float w) { return 1.0 - smoothstep(w * 0.55, w, abs(v - c)); }
float catSeg(vec3 p, vec3 a, vec3 b) { vec3 pa = p - a, ba = b - a; return length(pa - ba * clamp(dot(pa, ba) / dot(ba, ba), 0.0, 1.0)); }
vec3 catColor(vec3 p) {
  vec3 q = vec3(abs(p.x), p.y, p.z);
  float cream = catIn(p, vec3(0.0, 0.56, 0.2), vec3(0.18, 0.27, 0.16), 0.3);
  cream = max(cream, catIn(q, vec3(0.0, 1.13, 0.41), vec3(0.2, 0.13, 0.13), 0.35));
  cream = max(cream, catIn(q, vec3(0.48, 0.6, 0.06), vec3(0.1, 0.1, 0.11), 0.3));
  cream = max(cream, 1.0 - smoothstep(0.1, 0.13, p.y));
  float st = 0.0;
  float fh = smoothstep(1.47, 1.53, p.y) * smoothstep(0.1, 0.2, p.z);
  float w = mix(0.012, 0.03, smoothstep(1.5, 1.68, p.y));
  st = max(st, fh * max(catBand(p.x, 0.0, w), max(catBand(q.x, 0.085, w * 0.85), catBand(q.x, 0.165, w * 0.7))));
  float ch = smoothstep(0.37, 0.43, q.x) * smoothstep(0.0, 0.08, p.z);
  st = max(st, ch * max(catBand(p.y, 1.225, 0.014), catBand(p.y, 1.17, 0.012)));
  float nape = (1.0 - smoothstep(-0.12, 0.02, p.z)) * smoothstep(1.15, 1.3, p.y);
  st = max(st, nape * max(catBand(p.x, 0.0, 0.035), max(catBand(q.x, 0.11, 0.03), catBand(q.x, 0.22, 0.025))));
  float back = smoothstep(0.3, 0.36, p.y) * (1.0 - smoothstep(0.9, 0.96, p.y)) * (1.0 - smoothstep(-0.06, 0.06, p.z));
  st = max(st, back * smoothstep(0.55, 0.8, sin(p.y * 40.0 - q.x * 9.0)));
  float arm = smoothstep(0.27, 0.31, q.x) * smoothstep(0.62, 0.66, p.y) * (1.0 - smoothstep(0.88, 0.92, p.y));
  st = max(st, arm * smoothstep(0.65, 0.85, sin((q.x - p.y) * 38.0)));
  float leg = smoothstep(0.14, 0.18, p.y) * (1.0 - smoothstep(0.34, 0.38, p.y)) * smoothstep(0.15, 0.2, q.x);
  st = max(st, leg * smoothstep(0.65, 0.85, sin(p.y * 48.0)));
  float ear = 1.0 - smoothstep(0.055, 0.08, catSeg(q, vec3(0.26, 1.6, 0.0), vec3(0.37, 1.88, -0.03)));
  float pink = ear * smoothstep(1.64, 1.7, p.y) * (1.0 - smoothstep(1.8, 1.85, p.y)) * smoothstep(-0.02, 0.01, p.z);
  float pyat = smoothstep(0.56, 0.62, catNoise(p * 3.2 + uSid)) * uPyatnaVes;
  float pyat2 = smoothstep(0.6, 0.66, catNoise(p * 3.7 + uSid + 17.0)) * uPyatna2Ves;
  vec3 col = mix(uOsnova, uPyatno, pyat);
  col = mix(col, uPyatno2, pyat2);
  col = mix(col, uPolosy, st * uPolosyVes);
  col = mix(col, uSvetlyy, cream * uSvetlyyVes);
  col = mix(col, uRozovyy, pink);
  float n = catNoise(p * 95.0) * 0.55 + catNoise(p * 30.0) * 0.45;
  return col * (0.93 + 0.14 * n);
}
`;

// ---------- расстояния до простых тел (отрицательно внутри) ----------
function sdEll(x, y, z, c, r) {
  const px = (x - c[0]) / r[0], py = (y - c[1]) / r[1], pz = (z - c[2]) / r[2];
  const k0 = Math.sqrt(px * px + py * py + pz * pz);
  const qx = px / r[0], qy = py / r[1], qz = pz / r[2];
  const k1 = Math.sqrt(qx * qx + qy * qy + qz * qz);
  return k1 < 1e-9 ? -Math.min(r[0], r[1], r[2]) : (k0 * (k0 - 1)) / k1;
}

function sdCap(x, y, z, a, b, r) {
  const pax = x - a[0], pay = y - a[1], paz = z - a[2];
  const bax = b[0] - a[0], bay = b[1] - a[1], baz = b[2] - a[2];
  const h = Math.min(1, Math.max(0, (pax * bax + pay * bay + paz * baz) / (bax * bax + bay * bay + baz * baz)));
  const dx = pax - bax * h, dy = pay - bay * h, dz = paz - baz * h;
  return Math.sqrt(dx * dx + dy * dy + dz * dz) - r;
}

// Конус со скруглёнными концами: радиус r1 у точки a, r2 у точки b
function sdRoundCone(x, y, z, a, b, r1, r2) {
  const bax = b[0] - a[0], bay = b[1] - a[1], baz = b[2] - a[2];
  const l2 = bax * bax + bay * bay + baz * baz, rr = r1 - r2, a2 = l2 - rr * rr, il2 = 1 / l2;
  const pax = x - a[0], pay = y - a[1], paz = z - a[2];
  const yy = pax * bax + pay * bay + paz * baz, zz = yy - l2;
  const wx = pax * l2 - bax * yy, wy = pay * l2 - bay * yy, wz = paz * l2 - baz * yy;
  const x2 = wx * wx + wy * wy + wz * wz, y2 = yy * yy * l2, z2 = zz * zz * l2;
  const k = Math.sign(rr) * rr * rr * x2;
  if (Math.sign(zz) * a2 * z2 > k) return Math.sqrt(x2 + z2) * il2 - r2;
  if (Math.sign(yy) * a2 * y2 < k) return Math.sqrt(x2 + y2) * il2 - r1;
  return (Math.sqrt(x2 * a2 * il2) + yy * rr) * il2 - r1;
}

// Плавное слияние и плавное вычитание, k ширина перехода
function smin(a, b, k) {
  const h = Math.max(k - Math.abs(a - b), 0) / k;
  return Math.min(a, b) - h * h * k * 0.25;
}
const smax = (a, b, k) => -smin(-a, -b, k);

// ---------- скелет в покое: кот стоит, руки разведены на 45 градусов ----------
const ARM_REST = Math.PI / 4;
export const J = { pelvis: [0, 0.45, 0], spine: [0, 0.62, 0], head: [0, 0.95, 0.02] };
for (const [k, s] of [['L', 1], ['R', -1]]) {
  J[`ear${k}`] = [s * 0.26, 1.6, 0.0];
  J[`earTip${k}`] = [s * 0.37, 1.88, -0.03];
  J[`shoulder${k}`] = [s * 0.23, 0.86, 0.04];
  J[`elbow${k}`] = [s * 0.35, 0.74, 0.04];
  J[`wrist${k}`] = [s * 0.44, 0.645, 0.05];
  J[`hip${k}`] = [s * 0.13, 0.42, 0.03];
  J[`knee${k}`] = [s * 0.14, 0.24, 0.05];
  J[`ankle${k}`] = [s * 0.14, 0.09, 0.05];
  J[`eye${k}`] = [s * 0.19, 1.29, 0.365];
}
export const EYE_R = 0.13;

// Простые тела формы. bone: к какой кости тянутся вершины этого тела
function makePrims() {
  const P = [];
  const add = (bone, box, f) => P.push({ bone, box, f }) - 1;
  const ellBox = (c, r) => [c[0] - r[0], c[1] - r[1], c[2] - r[2], c[0] + r[0], c[1] + r[1], c[2] + r[2]];
  const capBox = (a, b, r) => [Math.min(a[0], b[0]) - r, Math.min(a[1], b[1]) - r, Math.min(a[2], b[2]) - r,
    Math.max(a[0], b[0]) + r, Math.max(a[1], b[1]) + r, Math.max(a[2], b[2]) + r];
  const ell = (bone, c, r) => add(bone, ellBox(c, r), (x, y, z) => sdEll(x, y, z, c, r));
  const cap = (bone, a, b, r) => add(bone, capBox(a, b, r), (x, y, z) => sdCap(x, y, z, a, b, r));

  const I = {};
  I.pelvis = ell('pelvis', [0, 0.5, 0.02], [0.26, 0.245, 0.23]);
  I.chest = ell('spine', [0, 0.72, 0.02], [0.2, 0.2, 0.18]);
  I.neck = ell('spine', [0, 0.92, 0.03], [0.15, 0.12, 0.14]);
  I.skull = ell('head', [0, 1.3, 0.03], [0.5, 0.42, 0.44]);
  for (const [k, s] of [['L', 1], ['R', -1]]) {
    I[`cheek${k}`] = ell('head', [s * 0.29, 1.17, 0.17], [0.2, 0.15, 0.17]);
    I[`muzzle${k}`] = ell('head', [s * 0.068, 1.16, 0.4], [0.085, 0.085, 0.085]);
    I[`sock${k}`] = ell(null, J[`eye${k}`], [EYE_R + 0.007, EYE_R + 0.007, EYE_R + 0.007]);
    // Ухо: скруглённый конус, сплющенный вперёд-назад
    const a = J[`ear${k}`], b = J[`earTip${k}`], F = 0.45;
    const aS = [a[0], a[1], a[2] / F], bS = [b[0], b[1], b[2] / F];
    I[`ear${k}`] = add(`ear${k}`, capBox(a, b, 0.15), (x, y, z) => sdRoundCone(x, y, z / F, aS, bS, 0.15, 0.03) * F);
    I[`up${k}`] = cap(`arm${k}`, J[`shoulder${k}`], J[`elbow${k}`], 0.072);
    I[`fore${k}`] = cap(`fore${k}`, J[`elbow${k}`], J[`wrist${k}`], 0.066);
    I[`paw${k}`] = ell(`fore${k}`, [s * 0.475, 0.61, 0.06], [0.088, 0.082, 0.092]);
    I[`thigh${k}`] = cap(`thigh${k}`, J[`hip${k}`], J[`knee${k}`], 0.095);
    I[`shin${k}`] = cap(`shin${k}`, J[`knee${k}`], J[`ankle${k}`], 0.077);
    I[`foot${k}`] = ell(`foot${k}`, [s * 0.14, 0.055, 0.11], [0.095, 0.058, 0.14]);
  }
  I.chin = ell('head', [0, 1.1, 0.37], [0.07, 0.055, 0.06]);
  return { P, I };
}

// Сборка формы из тел: конечности сливаются только с туловищем, не друг с другом
function combine(D, I) {
  const torso = smin(smin(D[I.pelvis], D[I.chest], 0.12), D[I.neck], 0.08);
  let head = smin(smin(D[I.skull], D[I.cheekL], 0.1), D[I.cheekR], 0.1);
  head = smin(head, smin(smin(D[I.muzzleL], D[I.muzzleR], 0.03), D[I.chin], 0.03), 0.04);
  head = smax(head, -D[I.sockL], 0.025);
  head = smax(head, -D[I.sockR], 0.025);
  head = smin(smin(head, D[I.earL], 0.06), D[I.earR], 0.06);
  const core = smin(torso, head, 0.07);
  let out = core;
  for (const k of ['L', 'R']) {
    const arm = smin(smin(D[I[`up${k}`]], D[I[`fore${k}`]], 0.03), D[I[`paw${k}`]], 0.035);
    const leg = smin(smin(D[I[`thigh${k}`]], D[I[`shin${k}`]], 0.03), D[I[`foot${k}`]], 0.04);
    out = Math.min(out, smin(core, arm, 0.05), smin(core, leg, 0.07));
  }
  return out;
}

// Дальше запаса от коробки тела точное расстояние не нужно: хватает нижней
// оценки (расстояние до коробки), знак и переходы слияния не меняются
const MARGIN = 0.15;
function makeField() {
  const { P, I } = makePrims();
  const D = new Float64Array(P.length);
  const evalAll = (x, y, z) => {
    for (let i = 0; i < P.length; i++) {
      const b = P[i].box;
      const dx = Math.max(b[0] - x, 0, x - b[3]), dy = Math.max(b[1] - y, 0, y - b[4]), dz = Math.max(b[2] - z, 0, z - b[5]);
      const out = Math.sqrt(dx * dx + dy * dy + dz * dz);
      D[i] = out > MARGIN ? out : P[i].f(x, y, z);
    }
    return D;
  };
  return { P, I, D, evalAll, field: (x, y, z) => combine(evalAll(x, y, z), I) };
}

// Сетка поверхности: вершина в каждой клетке со сменой знака, четырёхугольник
// на каждом ребре решётки со сменой знака, потом вершины доводятся до
// поверхности и получают нормаль по наклону поля
function surfaceNets(field, lo, hi, h) {
  const nx = Math.ceil((hi[0] - lo[0]) / h) + 1, ny = Math.ceil((hi[1] - lo[1]) / h) + 1, nz = Math.ceil((hi[2] - lo[2]) / h) + 1;
  const F = new Float32Array(nx * ny * nz);
  const id = (i, j, k) => i + nx * (j + ny * k);
  for (let k = 0; k < nz; k++) for (let j = 0; j < ny; j++) for (let i = 0; i < nx; i++) {
    const x = lo[0] + i * h, y = lo[1] + j * h, z = lo[2] + k * h;
    const f = field(x, y, z);
    // Форма на краю рамки срезается плоско и оставляет дыру: такую рамку не принимаем
    if (f < 0 && (i === 0 || j === 0 || k === 0 || i === nx - 1 || j === ny - 1 || k === nz - 1)) {
      throw new Error(`форма кота вылезла за рамку сетки у (${x.toFixed(2)}, ${y.toFixed(2)}, ${z.toFixed(2)})`);
    }
    F[id(i, j, k)] = f;
  }
  const cx = nx - 1, cy = ny - 1, cz = nz - 1;
  const cid = (i, j, k) => i + cx * (j + cy * k);
  const cellV = new Int32Array(cx * cy * cz).fill(-1);
  const CORNER = [[0, 0, 0], [1, 0, 0], [0, 1, 0], [1, 1, 0], [0, 0, 1], [1, 0, 1], [0, 1, 1], [1, 1, 1]];
  const EDGE = [[0, 1], [2, 3], [4, 5], [6, 7], [0, 2], [1, 3], [4, 6], [5, 7], [0, 4], [1, 5], [2, 6], [3, 7]];
  const v = new Float64Array(8);
  const pos = [];
  for (let k = 0; k < cz; k++) for (let j = 0; j < cy; j++) for (let i = 0; i < cx; i++) {
    let mask = 0;
    for (let c = 0; c < 8; c++) {
      v[c] = F[id(i + CORNER[c][0], j + CORNER[c][1], k + CORNER[c][2])];
      if (v[c] < 0) mask |= 1 << c;
    }
    if (mask === 0 || mask === 255) continue;
    let sx = 0, sy = 0, sz = 0, n = 0;
    for (const [a, b] of EDGE) {
      if ((v[a] < 0) === (v[b] < 0)) continue;
      const t = v[a] / (v[a] - v[b]);
      sx += CORNER[a][0] + (CORNER[b][0] - CORNER[a][0]) * t;
      sy += CORNER[a][1] + (CORNER[b][1] - CORNER[a][1]) * t;
      sz += CORNER[a][2] + (CORNER[b][2] - CORNER[a][2]) * t;
      n++;
    }
    cellV[cid(i, j, k)] = pos.length / 3;
    pos.push(lo[0] + (i + sx / n) * h, lo[1] + (j + sy / n) * h, lo[2] + (k + sz / n) * h);
  }
  const idx = [];
  const quad = (a, b, c, d, ox, oy, oz) => {
    if (a < 0 || b < 0 || c < 0 || d < 0) return;
    const e1x = pos[3 * c] - pos[3 * a], e1y = pos[3 * c + 1] - pos[3 * a + 1], e1z = pos[3 * c + 2] - pos[3 * a + 2];
    const e2x = pos[3 * d] - pos[3 * b], e2y = pos[3 * d + 1] - pos[3 * b + 1], e2z = pos[3 * d + 2] - pos[3 * b + 2];
    const nxv = e1y * e2z - e1z * e2y, nyv = e1z * e2x - e1x * e2z, nzv = e1x * e2y - e1y * e2x;
    if (nxv * ox + nyv * oy + nzv * oz >= 0) idx.push(a, b, c, a, c, d);
    else idx.push(a, d, c, a, c, b);
  };
  for (let k = 0; k < nz; k++) for (let j = 0; j < ny; j++) for (let i = 0; i < nx; i++) {
    const v0 = F[id(i, j, k)], o = v0 < 0 ? 1 : -1;
    if (i < cx && j >= 1 && j < cy && k >= 1 && k < cz && (v0 < 0) !== (F[id(i + 1, j, k)] < 0)) {
      quad(cellV[cid(i, j - 1, k - 1)], cellV[cid(i, j, k - 1)], cellV[cid(i, j, k)], cellV[cid(i, j - 1, k)], o, 0, 0);
    }
    if (j < cy && i >= 1 && i < cx && k >= 1 && k < cz && (v0 < 0) !== (F[id(i, j + 1, k)] < 0)) {
      quad(cellV[cid(i - 1, j, k - 1)], cellV[cid(i, j, k - 1)], cellV[cid(i, j, k)], cellV[cid(i - 1, j, k)], 0, o, 0);
    }
    if (k < cz && i >= 1 && i < cx && j >= 1 && j < cy && (v0 < 0) !== (F[id(i, j, k + 1)] < 0)) {
      quad(cellV[cid(i - 1, j - 1, k)], cellV[cid(i, j - 1, k)], cellV[cid(i, j, k)], cellV[cid(i - 1, j, k)], 0, 0, o);
    }
  }
  const nrm = new Float32Array(pos.length);
  const e = h * 0.25;
  for (let n = 0; n < pos.length; n += 3) {
    let x = pos[n], y = pos[n + 1], z = pos[n + 2], gx = 0, gy = 0, gz = 0;
    for (let it = 0; it < 3; it++) {
      const d = field(x, y, z);
      gx = (field(x + e, y, z) - field(x - e, y, z)) / (2 * e);
      gy = (field(x, y + e, z) - field(x, y - e, z)) / (2 * e);
      gz = (field(x, y, z + e) - field(x, y, z - e)) / (2 * e);
      const g2 = gx * gx + gy * gy + gz * gz;
      if (it === 2 || g2 < 1e-12) break;
      // Шаг к поверхности не дальше полуклетки, чтобы не перепрыгнуть на соседнюю
      const s = Math.max(-h * 0.5, Math.min(h * 0.5, d / g2));
      x -= gx * s; y -= gy * s; z -= gz * s;
    }
    pos[n] = x; pos[n + 1] = y; pos[n + 2] = z;
    const gl = Math.hypot(gx, gy, gz) || 1;
    nrm[n] = gx / gl; nrm[n + 1] = gy / gl; nrm[n + 2] = gz / gl;
  }
  return { pos: new Float32Array(pos), nrm, idx: new Uint32Array(idx) };
}

const BONE_NAMES = ['root', 'pelvis', 'spine', 'head', 'earL', 'earR', 'armL', 'foreL', 'armR', 'foreR',
  'thighL', 'shinL', 'footL', 'thighR', 'shinR', 'footR'];

// Вес кости в вершине по близости к её телам: на поверхности тела 1,
// в шве между телами веса смешиваются, дальше 0.1 почти 0
const SKIN_SOFT = 0.018;
function skinWeights(pos, F) {
  const n = pos.length / 3;
  const sIdx = new Uint16Array(n * 4), sW = new Float32Array(n * 4);
  const acc = new Float64Array(BONE_NAMES.length);
  const boneOf = F.P.map((p) => (p.bone ? BONE_NAMES.indexOf(p.bone) : -1));
  for (let v = 0; v < n; v++) {
    acc.fill(0);
    const D = F.evalAll(pos[3 * v], pos[3 * v + 1], pos[3 * v + 2]);
    for (let i = 0; i < D.length; i++) if (boneOf[i] >= 0) acc[boneOf[i]] += Math.exp(-Math.max(D[i], 0) / SKIN_SOFT);
    const top = [...acc.keys()].sort((a, b) => acc[b] - acc[a]).slice(0, 4);
    const sum = top.reduce((s, b) => s + acc[b], 0) || 1;
    top.forEach((b, k) => {
      sIdx[v * 4 + k] = b;
      sW[v * 4 + k] = acc[b] / sum;
    });
  }
  return { sIdx, sW };
}

// ---------- мелкие детали ----------
function fur(opts) {
  return new THREE.MeshPhysicalMaterial({
    roughness: 0.82, sheen: 0.6, sheenRoughness: 0.45, sheenColor: new THREE.Color(0xffd2a0), ...opts,
  });
}

function mesh(geo, mat, x = 0, y = 0, z = 0, sx = 1, sy = 1, sz = 1) {
  const m = new THREE.Mesh(geo, mat);
  m.position.set(x, y, z);
  m.scale.set(sx, sy, sz);
  return m;
}

function group(parent, x = 0, y = 0, z = 0) {
  const g = new THREE.Group();
  g.position.set(x, y, z);
  parent.add(g);
  return g;
}

// Сужающаяся трубка хвоста, вершины пересчитываются каждый кадр
class TaperTube {
  constructor(n, radial, r0, r1, mat, tipMat, a, b) {
    this.n = n;
    this.radial = radial;
    this.r0 = r0;
    this.r1 = r1;
    const pos = new Float32Array(n * radial * 3);
    const col = new Float32Array(n * radial * 3);
    const idx = [];
    for (let i = 0; i < n; i++) {
      const s = i / (n - 1);
      const c = Math.sin(s * Math.PI * 7) > 0.55 ? b : a;
      for (let j = 0; j < radial; j++) col.set([c.r, c.g, c.b], (i * radial + j) * 3);
    }
    for (let i = 0; i < n - 1; i++) {
      for (let j = 0; j < radial; j++) {
        const p = i * radial + j, q = i * radial + ((j + 1) % radial);
        idx.push(p, p + radial, q, q, p + radial, q + radial);
      }
    }
    this.geo = new THREE.BufferGeometry();
    this.geo.setAttribute('position', new THREE.BufferAttribute(pos, 3));
    this.geo.setAttribute('color', new THREE.BufferAttribute(col, 3));
    this.geo.setIndex(idx);
    this.mesh = new THREE.Mesh(this.geo, mat);
    this.mesh.frustumCulled = false;
    // У сферы кончика нет цветов вершин, поэтому свой материал
    this.tip = new THREE.Mesh(new THREE.SphereGeometry(r1, 16, 12), tipMat);
  }

  update(pts) {
    const pos = this.geo.attributes.position.array;
    const t = new THREE.Vector3(), nrm = new THREE.Vector3(1, 0, 0), bin = new THREE.Vector3(), v = new THREE.Vector3();
    for (let i = 0; i < this.n; i++) {
      const a = pts[Math.max(0, i - 1)], b = pts[Math.min(this.n - 1, i + 1)];
      t.subVectors(b, a).normalize();
      // Перенос рамки вдоль кривой без скручивания
      nrm.sub(v.copy(t).multiplyScalar(nrm.dot(t))).normalize();
      bin.crossVectors(t, nrm);
      const s = i / (this.n - 1);
      const r = this.r0 + (this.r1 - this.r0) * s;
      for (let j = 0; j < this.radial; j++) {
        const ang = (j / this.radial) * Math.PI * 2;
        const k = (i * this.radial + j) * 3;
        pos[k] = pts[i].x + r * (Math.cos(ang) * nrm.x + Math.sin(ang) * bin.x);
        pos[k + 1] = pts[i].y + r * (Math.cos(ang) * nrm.y + Math.sin(ang) * bin.y);
        pos[k + 2] = pts[i].z + r * (Math.cos(ang) * nrm.z + Math.sin(ang) * bin.z);
      }
    }
    this.geo.attributes.position.needsUpdate = true;
    this.geo.computeVertexNormals();
    this.tip.position.copy(pts[this.n - 1]);
  }
}

const sub = (a, b) => [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
const v3 = (a) => new THREE.Vector3(a[0], a[1], a[2]);

// Сетка тела одна на страницу: строится около 5 секунд, коты её делят
let SHARED = null;
function sharedBody() {
  if (SHARED) return SHARED;
  const F = makeField();
  const t0 = performance.now();
  const S = surfaceNets(F.field, [-0.62, -0.03, -0.48], [0.62, 1.95, 0.6], 0.009);
  const { sIdx, sW } = skinWeights(S.pos, F);
  const geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.BufferAttribute(S.pos, 3));
  geo.setAttribute('normal', new THREE.BufferAttribute(S.nrm, 3));
  geo.setAttribute('skinIndex', new THREE.Uint16BufferAttribute(sIdx, 4));
  geo.setAttribute('skinWeight', new THREE.Float32BufferAttribute(sW, 4));
  geo.setIndex(new THREE.BufferAttribute(S.idx, 1));
  SHARED = { geo, buildMs: performance.now() - t0, vertexCount: S.pos.length / 3 };
  return SHARED;
}

export class Cat {
  // okras: поля OKRAS; slozhenie: { rost, golova, tolschina }, всё по умолчанию 1
  constructor({ okras = {}, slozhenie = {} } = {}) {
    const O = { ...OKRAS, ...okras };
    this.okras = O;
    const { geo, buildMs, vertexCount } = sharedBody();
    this.buildMs = buildMs;
    this.vertexCount = vertexCount;

    // Окрас: цвет по точке тела в покое (position до скиннинга). Программа
    // шейдера у всех котов одна, значения окраса у каждого свои
    const bodyMat = fur({ sheenColor: new THREE.Color(O.blesk) });
    const col = (hex) => ({ value: new THREE.Color(hex) });
    const uni = {
      uOsnova: col(O.osnova), uSvetlyy: col(O.svetlyy), uPolosy: col(O.polosy), uRozovyy: col(O.rozovyy),
      uPyatno: col(O.pyatno), uPyatno2: col(O.pyatno2), uPolosyVes: { value: O.polosyVes },
      uPyatnaVes: { value: O.pyatnaVes }, uPyatna2Ves: { value: O.pyatna2Ves },
      uSvetlyyVes: { value: O.svetlyyVes }, uSid: { value: O.sid },
    };
    bodyMat.onBeforeCompile = (sh) => {
      Object.assign(sh.uniforms, uni);
      sh.vertexShader = sh.vertexShader
        .replace('#include <common>', '#include <common>\nvarying vec3 vRest;')
        .replace('#include <begin_vertex>', '#include <begin_vertex>\nvRest = position;');
      sh.fragmentShader = sh.fragmentShader
        .replace('#include <common>', `#include <common>\nvarying vec3 vRest;\n${CAT_COLOR_GLSL}`)
        .replace('vec4 diffuseColor = vec4( diffuse, opacity );', 'vec4 diffuseColor = vec4( catColor( vRest ), opacity );');
      if (!sh.fragmentShader.includes('catColor( vRest )') || !sh.vertexShader.includes('vRest = position;')) {
        throw new Error('three поменял шейдер: окрас кота не встал на место');
      }
    };

    // Кости в позе покоя
    this.root = new THREE.Group();
    const B = {};
    const bone = (name, parent, at) => {
      const b = new THREE.Bone();
      b.name = name;
      const pw = parent ? parent.userData.at : [0, 0, 0];
      b.position.set(at[0] - pw[0], at[1] - pw[1], at[2] - pw[2]);
      b.userData.at = at;
      if (parent) parent.add(b);
      B[name] = b;
      return b;
    };
    bone('root', null, [0, 0, 0]);
    bone('pelvis', B.root, J.pelvis);
    bone('spine', B.pelvis, J.spine);
    bone('head', B.spine, J.head);
    for (const k of ['L', 'R']) {
      bone(`ear${k}`, B.head, J[`ear${k}`]);
      bone(`arm${k}`, B.spine, J[`shoulder${k}`]);
      bone(`fore${k}`, B[`arm${k}`], J[`elbow${k}`]);
      bone(`thigh${k}`, B.pelvis, J[`hip${k}`]);
      bone(`shin${k}`, B[`thigh${k}`], J[`knee${k}`]);
      bone(`foot${k}`, B[`shin${k}`], J[`ankle${k}`]);
    }
    this.B = B;
    this.body = new THREE.SkinnedMesh(geo, bodyMat);
    this.body.frustumCulled = false;
    this.body.castShadow = true;
    this.body.receiveShadow = true;
    this.body.add(B.root);
    this.root.add(this.body);
    this.body.bind(new THREE.Skeleton(BONE_NAMES.map((n) => B[n])));

    // Сложение: рост общий, толщина вширь и вглубь, голова отдельно
    const { rost = 1, golova = 1, tolschina = 1 } = slozhenie;
    this.root.scale.set(rost * tolschina, rost, rost * tolschina);
    B.head.scale.setScalar(golova);

    // Ноги: длины звеньев и их направления в покое
    this.legs = {};
    for (const [k, s] of [['L', 1], ['R', -1]]) {
      const thigh = sub(J[`knee${k}`], J[`hip${k}`]), shin = sub(J[`ankle${k}`], J[`knee${k}`]);
      this.legs[k] = {
        s, L1: Math.hypot(...thigh), L2: Math.hypot(...shin),
        restThigh: v3(thigh).normalize(), restShin: v3(shin).normalize(),
      };
    }
    // Руки смоделированы разведёнными на 45 градусов, каналы считают подъём от опущенной
    this.armRest = {};
    for (const [k, s] of [['L', 1], ['R', -1]]) {
      const rest = new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0, 0, 1), s * ARM_REST);
      this.armRest[k] = { s, rest, restInv: rest.clone().invert() };
    }

    this.buildFace(B.head);

    // Хвост от крестца: кольца цвета полосок, у однотонного кота их нет
    const blesk = new THREE.Color(O.blesk);
    const tailMat = fur({ vertexColors: true, color: 0xffffff, sheenColor: blesk });
    const ringA = new THREE.Color(O.osnova);
    const ringB = ringA.clone().lerp(new THREE.Color(O.polosy), O.polosyVes);
    this.tail = new TaperTube(26, 18, 0.062, 0.036, tailMat, fur({ color: ringB, sheenColor: blesk }), ringA, ringB);
    this.tail.mesh.castShadow = this.tail.tip.castShadow = true;
    B.pelvis.add(this.tail.mesh, this.tail.tip);
    this.tailPts = Array.from({ length: 26 }, () => new THREE.Vector3());

    this._v = Array.from({ length: 6 }, () => new THREE.Vector3());
    this._q = Array.from({ length: 6 }, () => new THREE.Quaternion());
    this._e = new THREE.Euler();
    // Каналы рук из последнего update(P) и кисть в осях предплечья: для dotyanut
    this._poza = { L: [0, 0, 0, 0], R: [0, 0, 0, 0] };
    this._kist = { L: v3(sub(J.wristL, J.elbowL)), R: v3(sub(J.wristR, J.elbowR)) };
  }

  // Повесить предмет на кость. at: где он у кота в позе покоя (как J)
  prikrepit(kost, obj, at = [0, 0, 0]) {
    const b = this.B[kost];
    if (!b) throw new Error(`у кота нет кости ${kost}`);
    const r = b.userData.at;
    obj.position.set(at[0] - r[0], at[1] - r[1], at[2] - r[2]);
    b.add(obj);
    return obj;
  }

  // Глаза с белками и веками, нос, рот, усы: всё на кости головы
  buildFace(head) {
    const at = (w) => sub(w, J.head);
    const glossy = (color) => new THREE.MeshPhysicalMaterial({ color, roughness: 0.1, clearcoat: 1, clearcoatRoughness: 0.06 });
    const shine = new THREE.MeshBasicMaterial({ color: 0xffffff });
    const lidMat = fur({ color: this.okras.osnova, sheenColor: new THREE.Color(this.okras.blesk) });
    const lash = new THREE.MeshStandardMaterial({ color: 0x2a140c, roughness: 0.6 });
    const cap = (r, from, len) => new THREE.SphereGeometry(r, 48, 24, 0, Math.PI * 2, from, len);
    this.eyes = [];
    for (const [k, s] of [['L', 1], ['R', -1]]) {
      const eye = group(head, ...at(J[`eye${k}`]));
      eye.rotation.y = s * 0.3;
      eye.add(mesh(new THREE.SphereGeometry(EYE_R, 48, 32), glossy(0xfbf7f2)));
      // Радужка и зрачок шапочками на яблоке, поворачиваются вместе со взглядом
      const look = group(eye);
      // Крупные радужка и зрачок: белка мало, взгляд милый, а не тревожный
      const iris = mesh(cap(EYE_R * 1.004, 0, 0.98), glossy(this.okras.glaza));
      const pupil = mesh(cap(EYE_R * 1.008, 0, 0.68), glossy(0x0b0b12));
      iris.rotation.x = pupil.rotation.x = Math.PI / 2;
      look.add(iris, pupil);
      // Блики с одной стороны у обоих глаз: свет один
      // Блики торчат сквозь сомкнутое веко, поэтому при закрытом глазе прячутся
      const highlights = [];
      const hl = (dx, dy, r) => {
        const d = new THREE.Vector3(dx, dy, 0.85).normalize().multiplyScalar(EYE_R * 1.02);
        const m = mesh(new THREE.SphereGeometry(r, 16, 12), shine, d.x, d.y, d.z);
        eye.add(m);
        highlights.push(m);
      };
      hl(-0.38, 0.42, 0.03);
      hl(0.3, -0.3, 0.014);
      // Веки: верхнее и нижнее полусферой чуть больше яблока, наклон общий
      const tilt = group(eye);
      const upper = mesh(cap(EYE_R * 1.07, 0, Math.PI / 2), lidMat);
      const lower = mesh(cap(EYE_R * 1.065, Math.PI / 2, Math.PI / 2), lidMat);
      tilt.add(upper, lower);
      // Линия сомкнутого века: дуга, видна, когда глаз закрыт
      const line = mesh(new THREE.TorusGeometry(EYE_R * 0.62, 0.008, 8, 32, Math.PI), lash, 0, -0.005, EYE_R * 1.04);
      tilt.add(line);
      this.eyes.push({ eye, look, tilt, upper, lower, line, highlights, s });
    }

    const nose = new THREE.MeshPhysicalMaterial({ color: 0xff8fa3, roughness: 0.35, clearcoat: 0.6 });
    head.add(mesh(new THREE.SphereGeometry(0.042, 24, 16), nose, ...at([0, 1.225, 0.468]), 1.3, 0.75, 0.8));
    // Рот: две дужки буквой w и открытая часть, которая растёт по каналу mouth
    const lip = new THREE.MeshStandardMaterial({ color: 0x4a1f16, roughness: 0.6 });
    this.lips = [];
    for (const s of [1, -1]) {
      // Поверхность морды здесь около z 0.48: плавное слияние добавляет объёма
      const arc = mesh(new THREE.TorusGeometry(0.034, 0.0075, 8, 20, Math.PI), lip, ...at([s * 0.034, 1.166, 0.486]));
      head.add(arc);
      this.lips.push({ arc, s });
    }
    this.mouthOpen = group(head, ...at([0, 1.12, 0.45]));
    this.mouthOpen.add(mesh(new THREE.SphereGeometry(0.06, 24, 16), new THREE.MeshStandardMaterial({ color: 0x3a1018, roughness: 0.7 }), 0, 0, 0, 1, 1, 0.35));
    this.mouthOpen.add(mesh(new THREE.SphereGeometry(0.036, 16, 12), new THREE.MeshStandardMaterial({ color: 0xff7f96, roughness: 0.5 }), 0, -0.026, 0.014, 1.1, 0.6, 0.5));
    const wh = new THREE.MeshBasicMaterial({ color: 0xfff6ea });
    for (const s of [1, -1]) {
      for (let i = 0; i < 2; i++) {
        const w = mesh(new THREE.CylinderGeometry(0.0032, 0.0018, 0.28, 6), wh, ...at([s * 0.27, 1.15 - i * 0.035, 0.36]));
        w.rotation.set(0, -s * 0.35, s * (Math.PI / 2 + (i - 0.5) * 0.2));
        head.add(w);
      }
    }
  }

  solveLeg(leg, k, target, pole, P) {
    const B = this.B;
    const [hip, d, v, knee, ankle] = this._v;
    const [qRootInv, qParent, qt, qs, qf] = this._q;
    this.root.updateMatrixWorld(true);
    qRootInv.copy(this.root.quaternion).invert();
    B[`thigh${k}`].getWorldPosition(hip);
    this.root.worldToLocal(hip);
    d.subVectors(target, hip);
    const len = Math.min(leg.L1 + leg.L2 - 1e-3, Math.max(Math.abs(leg.L1 - leg.L2) + 1e-3, d.length()));
    d.normalize();
    const cosA = (leg.L1 ** 2 + len ** 2 - leg.L2 ** 2) / (2 * leg.L1 * len);
    const a = Math.acos(Math.min(1, Math.max(-1, cosA)));
    v.copy(pole).addScaledVector(d, -pole.dot(d)).normalize();
    knee.copy(hip).addScaledVector(d, Math.cos(a) * leg.L1).addScaledVector(v, Math.sin(a) * leg.L1);
    ankle.copy(hip).addScaledVector(d, len);
    // Повороты костей в пространстве кота, затем в пространстве родителя
    B.pelvis.getWorldQuaternion(qParent).premultiply(qRootInv);
    qt.setFromUnitVectors(leg.restThigh, v.subVectors(knee, hip).normalize());
    B[`thigh${k}`].quaternion.copy(qParent).invert().multiply(qt);
    qs.setFromUnitVectors(leg.restShin, v.subVectors(ankle, knee).normalize());
    B[`shin${k}`].quaternion.copy(qt).invert().multiply(qs);
    qf.setFromEuler(this._e.set(P[`foot${k}_pitch`], leg.s * 0.1, 0));
    B[`foot${k}`].quaternion.copy(qs).invert().multiply(qf);
  }

  update(P) {
    const B = this.B;
    this.root.position.x = P.x;
    this.root.rotation.y = P.yaw;
    // В покое кость таза на 0.45, каналы считают от 0.49: колени чуть согнуты
    B.pelvis.position.set(P.pelvisX, P.pelvisY - 0.06, 0);
    B.pelvis.rotation.set(P.pitch, 0, P.lean + P.hipSway);
    // Сплющивание на груди и голове: ноги не тянутся, решение ног честное
    const q = Math.max(0.5, P.squash);
    B.spine.scale.set(1 / Math.sqrt(q), q, 1 / Math.sqrt(q));
    B.spine.rotation.set(P.chestBend, P.twist, P.chestSide - P.hipSway * 0.7);
    B.head.rotation.set(P.headNod, P.headTurn, P.headTilt);
    for (const k of ['L', 'R']) {
      const s = k === 'L' ? 1 : -1;
      B[`ear${k}`].rotation.set(-0.55 * P.earBack + 0.25 * P.earFlop, 0, -s * 0.45 * P.earBack);
    }

    for (const k of ['L', 'R']) {
      const r = this._poza[k];
      r[0] = P[`arm${k}_lift`]; r[1] = P[`arm${k}_fwd`]; r[2] = P[`arm${k}_elbow`]; r[3] = P[`arm${k}_elbowZ`];
      this.postavitRuku(k, ...r);
    }

    const pole = this._pole ?? (this._pole = new THREE.Vector3());
    const target = this._target ?? (this._target = new THREE.Vector3());
    for (const k of ['L', 'R']) {
      const leg = this.legs[k];
      target.set(P[`foot${k}_x`], P[`foot${k}_y`], P[`foot${k}_z`]);
      pole.set(leg.s * 0.25, 0, 1);
      this.solveLeg(leg, k, target, pole, P);
    }

    // Хвост: изгиб вверх и волна от основания к кончику
    const pts = this.tailPts;
    pts[0].set(0, 0.05, -0.19);
    const seg = 0.034;
    for (let i = 1; i < pts.length; i++) {
      const s = i / (pts.length - 1);
      const pitch = -0.35 + 1.9 * s ** 1.3 + P.tailLift * s;
      const yaw = P.tailAmp * s ** 1.2 * Math.sin(P.tailPhase - 2.6 * s) + P.tailSide * s;
      pts[i].set(
        pts[i - 1].x + seg * Math.sin(yaw) * Math.cos(pitch),
        pts[i - 1].y + seg * Math.sin(pitch),
        pts[i - 1].z - seg * Math.cos(yaw) * Math.cos(pitch),
      );
    }
    this.tail.update(pts);

    // Веки: открыто верхнее откинуто назад, нижнее опущено
    const blink = [P.blinkL, P.blinkR];
    const lerp = (a, b, k) => a + (b - a) * k;
    this.eyes.forEach((e, i) => {
      const close = Math.min(1, Math.max(blink[i], P.happy));
      const squint = Math.min(1, Math.max(P.squint, P.happy));
      // Закрываясь, верхнее веко опускается, а нижнее встаёт ему навстречу,
      // иначе под сомкнутым верхним остаётся полоска глаза
      e.upper.rotation.x = lerp(-0.95, 0.14, close);
      e.lower.rotation.x = lerp(lerp(0.95, -0.12, squint), -0.05, close);
      // Решительно (-1): внутренние уголки ниже; жалобно (+1): выше
      e.tilt.rotation.z = e.s * 0.3 * P.lidTilt;
      e.line.visible = close > 0.85;
      for (const h of e.highlights) h.visible = close < 0.5;
      // Зажмурился от радости: дуга вверх, просто моргнул: дуга вниз
      e.line.rotation.z = P.happy > 0.5 ? 0 : Math.PI;
      e.line.position.y = P.happy > 0.5 ? -0.02 : -0.004;
      e.look.rotation.set(-0.25 * P.lookY, -e.s * 0.22 + 0.3 * P.lookX, 0);
    });
    for (const l of this.lips) l.arc.rotation.set(0.25, 0, Math.PI + l.s * 0.35 * (P.smile - 0.5));
    const m = Math.max(0, Math.min(1, P.mouth));
    this.mouthOpen.scale.set(0.6 + 0.5 * m, Math.max(0.05, m), 1);
    this.mouthOpen.visible = m > 0.04;
  }

  // Рука по каналам позы: тот же смысл, что у опущенной руки, пересчитанный в позу покоя
  postavitRuku(k, lift, fwd, elbow, elbowZ) {
    const [qUp, qEl] = this._q;
    const { s, rest, restInv } = this.armRest[k];
    qUp.setFromEuler(this._e.set(-fwd, 0, s * lift));
    this.B[`arm${k}`].quaternion.copy(qUp).multiply(restInv);
    qEl.setFromEuler(this._e.set(-elbow, 0, s * elbowZ));
    this.B[`fore${k}`].quaternion.copy(rest).multiply(qEl).multiply(restInv);
  }

  // Где кисть лапы k в мире. Матрицы костей должны быть свежими
  kist(k, out) {
    return this.B[`fore${k}`].localToWorld(out.copy(this._kist[k]));
  }

  // Дотянуть кисть лапы k до точки мира cel. Вызывать после update(P): от
  // каналов позы подбирает подъём, вынос и сгиб (разворот локтя остаётся из
  // позы), ves смешивает позу и найденное. Повторный вызов до следующего
  // update(P) снова считает от позы, а не от прошлого решения. Возвращает, на
  // сколько кисть не достала бы при полном весе: лапы у кота короткие
  dotyanut(k, cel, ves = 1) {
    const nach = this._poza[k].slice();
    const arm = this.B[`arm${k}`];
    // Свежие матрицы нужны только цепочке над рукой, саму руку пересчитывает ostatok
    arm.parent.updateWorldMatrix(true, false);
    const [r, rh, j0, j1, j2] = this._dot ?? (this._dot = Array.from({ length: 5 }, () => new THREE.Vector3()));
    const Jc = [j0, j1, j2];
    const ostatok = (p, out) => {
      this.postavitRuku(k, p[0], p[1], p[2], nach[3]);
      arm.updateMatrixWorld(true);
      return this.kist(k, out).sub(cel);
    };
    // Подъём, вынос, сгиб в пределах, где рука кота ещё выглядит рукой
    const PREDEL = [[-0.4, 2.6], [-0.6, 2.2], [0, 2.4]];
    const vPredele = (v, i) => Math.min(PREDEL[i][1], Math.max(PREDEL[i][0], v));
    let x = nach.slice(0, 3).map(vPredele);
    let lam = 1e-3;
    const h = 1e-3;
    ostatok(x, r);
    // Левенберг-Марквардт: шаг (JᵀJ + λ) d = -Jᵀr, удачный шаг уменьшает λ
    for (let it = 0; it < 14 && r.lengthSq() > 1e-8; it++) {
      for (let i = 0; i < 3; i++) {
        const p = x.slice();
        p[i] += h;
        Jc[i].copy(ostatok(p, rh)).sub(r).divideScalar(h);
      }
      const A = Jc.map((a, i) => Jc.map((b, j) => a.dot(b) + (i === j ? lam : 0)));
      const d = reshit3(A, Jc.map((a) => -a.dot(r)));
      const xn = x.map((v, i) => vPredele(v + d[i], i));
      ostatok(xn, rh);
      if (rh.lengthSq() < r.lengthSq()) {
        x = xn;
        r.copy(rh);
        lam *= 0.3;
      } else {
        lam *= 10;
      }
    }
    const promah = r.length();
    this.postavitRuku(k, ...x.map((v, i) => nach[i] + (v - nach[i]) * ves), nach[3]);
    arm.updateMatrixWorld(true);
    return promah;
  }
}

// Система 3×3 по Крамеру: у JᵀJ + λ с λ > 0 определитель положительный
function reshit3(A, b) {
  const det = (m) => m[0][0] * (m[1][1] * m[2][2] - m[1][2] * m[2][1])
    - m[0][1] * (m[1][0] * m[2][2] - m[1][2] * m[2][0])
    + m[0][2] * (m[1][0] * m[2][1] - m[1][1] * m[2][0]);
  const D = det(A);
  return [0, 1, 2].map((i) => det(A.map((row, r) => row.map((v, c) => (c === i ? b[r] : v)))) / D);
}
