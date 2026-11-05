// Танцующий кот на подиуме визитки. Настройки (окрас, танцы, темп, свет)
// сервер подставляет строкой JSON вместо METKA при отдаче файла, поэтому у
// каждого сайта файл свой, а собранный код один.
import * as THREE from 'three';
import { Cat, REST } from './kot.js';
import { TANCY, mixPose, ss, bump, frac, clamp } from './tancy.js';

const METKA = '__NASTROYKA_KOTA__';

// Кадр с запасом на прыжки, поднятые лапы и шаги в стороны
const VERH = 2.25, NIZ = -0.05, SHIR = 1.5;
const FRAZA = 16;

// Адрес почты собирается из имени сайта: на адресе IP его не показываем
function pochta(n) {
  const blok = document.getElementById(n.pochtaId);
  const imya = location.hostname.replace(/\.$/, '').replace(/^www\./, '');
  if (!blok || !imya.includes('.') || /^[\d.]+$/.test(imya) || imya.includes(':')) return;
  const ssylka = blok.querySelector('a');
  if (!ssylka) return;
  const adres = `${n.pochta}@${imya}`;
  ssylka.href = `mailto:${adres}`;
  ssylka.textContent = adres;
  blok.hidden = false;
}

// Свой мягкий свет для отражений шерсти и глаз, без готовых картинок
function okruzhenie(renderer, svet) {
  const pm = new THREE.PMREMGenerator(renderer);
  const sc = new THREE.Scene();
  sc.background = new THREE.Color(svet.zemlya);
  const panel = (cvet, x, y, z, w, h) => {
    const m = new THREE.Mesh(
      new THREE.PlaneGeometry(w, h),
      new THREE.MeshBasicMaterial({ color: new THREE.Color(cvet).multiplyScalar(3), side: THREE.DoubleSide }),
    );
    m.position.set(x, y, z);
    m.lookAt(0, 0, 0);
    sc.add(m);
  };
  panel(svet.klyuch, 3, 6, 5, 4, 3);
  panel(svet.kontur, -6, 2, -4, 3, 2);
  panel(svet.nebo, -3, 5, 6, 2, 2);
  const tex = pm.fromScene(sc, 0.04).texture;
  pm.dispose();
  return tex;
}

// Поворот к одному обороту: после полного круга угол 2π, и смешивание с
// другим танцем иначе раскрутило бы кота обратно
const odinOborot = (a) => a - 2 * Math.PI * Math.round(a / (2 * Math.PI));

function zapusk(n) {
  const podium = document.getElementById(n.id);
  const holst = podium && podium.querySelector('canvas');
  if (!holst) return;

  let renderer;
  try {
    renderer = new THREE.WebGLRenderer({ canvas: holst, antialias: true, alpha: true, powerPreference: 'low-power' });
  } catch (oshibka) {
    // WebGL нет или он выключен: на подиуме остаётся рисунок кота
    podium.dataset.bezObema = String(oshibka && oshibka.message ? oshibka.message : oshibka);
    return;
  }
  renderer.setClearColor(0x000000, 0);
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.0;
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.shadowMap.enabled = true;

  const scene = new THREE.Scene();
  scene.environment = okruzhenie(renderer, n.svet);
  scene.environmentIntensity = 0.5;

  const kamera = new THREE.PerspectiveCamera(28, 1, 0.1, 60);
  scene.add(new THREE.HemisphereLight(n.svet.nebo, n.svet.zemlya, 1.1));
  const klyuch = new THREE.DirectionalLight(n.svet.klyuch, 2.2);
  klyuch.position.set(2.2, 4, 3);
  klyuch.castShadow = true;
  klyuch.shadow.mapSize.set(1024, 1024);
  Object.assign(klyuch.shadow.camera, { left: -1.6, right: 1.6, top: 2.6, bottom: -0.6, near: 0.5, far: 12 });
  klyuch.shadow.normalBias = 0.02;
  const kontur = new THREE.DirectionalLight(n.svet.kontur, 1.4);
  kontur.position.set(-3, 2.5, -2.5);
  scene.add(klyuch, kontur);

  const pol = new THREE.Mesh(new THREE.CircleGeometry(1.6, 48), new THREE.ShadowMaterial({ opacity: 0.22 }));
  pol.rotation.x = -Math.PI / 2;
  pol.receiveShadow = true;
  scene.add(pol);

  const kot = new Cat({ okras: n.okras, slozhenie: n.slozhenie });
  const opora = new THREE.Group();
  opora.rotation.y = n.povorot;
  opora.add(kot.root);
  scene.add(opora);

  const tancy = n.tancy.map((imya) => TANCY[imya] || TANCY.kach);
  const doley = n.temp / 60;
  const vzglyad = { x: 0, y: 0 }, cel = { x: 0, y: 0 };
  const tiho = matchMedia('(prefers-reduced-motion: reduce)');
  const nachalo = performance.now();
  let viden = true, kadrId = 0, gotov = false, poteryan = false, slomano = false;

  function kadr() {
    const w = podium.clientWidth, h = podium.clientHeight;
    if (!w || !h) return false;
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    renderer.setSize(w, h, false);
    kamera.aspect = w / h;
    const t = Math.tan((kamera.fov * Math.PI) / 360);
    const vys = (VERH - NIZ) * n.zapas;
    const dal = Math.max(vys / (2 * t), (SHIR * n.zapas) / (2 * t * kamera.aspect));
    const cy = (VERH + NIZ) / 2;
    kamera.position.set(0, cy + n.kamera, dal);
    kamera.lookAt(0, cy, 0);
    kamera.updateProjectionMatrix();
    return true;
  }

  // Две манеры по очереди, каждая по фразе в 16 долей: первая тише, вторая
  // во всю силу. Последнюю долю фразы одна плавно переходит в другую
  function poza(sek) {
    const p = sek * doley;
    const u = ((p % (2 * FRAZA)) + 2 * FRAZA) % (2 * FRAZA);
    const pervaya = u < FRAZA;
    const [tek, sled] = pervaya ? [tancy[0], tancy[1]] : [tancy[1], tancy[0]];
    const sila = pervaya ? 0.7 : 1;
    const A = tek(p, { sila });
    A.yaw = odinOborot(A.yaw);
    const k = ss(FRAZA - 1, FRAZA, u % FRAZA);
    let P = A;
    if (k > 0) {
      const B = sled(p, { sila: pervaya ? 1 : 0.7 });
      B.yaw = odinOborot(B.yaw);
      P = mixPose(A, B, k);
    }
    // Моргает раз в несколько секунд, не в такт музыке
    P.blinkL = P.blinkR = Math.max(P.blinkL, bump(frac(sek / 3.7), 0.5, 0.025));
    P.lookX = vzglyad.x;
    P.lookY = vzglyad.y;
    P.headTurn += 0.3 * vzglyad.x;
    P.headNod -= 0.1 * vzglyad.y;
    return P;
  }

  function narisovat(P) {
    if (poteryan || slomano) return;
    try {
      kot.update(P);
      renderer.render(scene, kamera);
    } catch (oshibka) {
      // Видеокарта или three не справились: дальше не рисуем, остаётся рисунок
      slomano = true;
      stop();
      podium.classList.remove('zhivoy');
      podium.dataset.bezObema = String(oshibka && oshibka.message ? oshibka.message : oshibka);
      return;
    }
    if (!gotov) {
      gotov = true;
      podium.classList.add('zhivoy');
    }
  }

  function tik(seychas) {
    kadrId = 0;
    if (!viden || tiho.matches) return;
    vzglyad.x += (cel.x - vzglyad.x) * 0.06;
    vzglyad.y += (cel.y - vzglyad.y) * 0.06;
    narisovat(poza((seychas - nachalo) / 1000));
    kadrId = requestAnimationFrame(tik);
  }

  function pusk() {
    if (!kadrId && viden && !poteryan && !slomano && !tiho.matches) kadrId = requestAnimationFrame(tik);
  }

  function stop() {
    if (kadrId) cancelAnimationFrame(kadrId);
    kadrId = 0;
  }

  // Без движения кот стоит в позе покоя и улыбается
  function stoit() {
    stop();
    narisovat({ ...REST, smile: 0.9 });
  }

  function obnovit() {
    if (!kadr()) return;
    if (tiho.matches) stoit();
    else if (!kadrId) narisovat(poza((performance.now() - nachalo) / 1000));
  }

  new ResizeObserver(obnovit).observe(podium);
  new IntersectionObserver((zapisi) => {
    viden = zapisi[zapisi.length - 1].isIntersecting;
    if (viden) pusk();
    else stop();
  }).observe(podium);
  tiho.addEventListener('change', () => (tiho.matches ? stoit() : pusk()));
  window.addEventListener('pointermove', (e) => {
    const r = podium.getBoundingClientRect();
    cel.x = clamp(((e.clientX - (r.left + r.width / 2)) / window.innerWidth) * 2, -1, 1);
    cel.y = clamp(((r.top + r.height * 0.35 - e.clientY) / window.innerHeight) * 2, -1, 1);
  }, { passive: true });
  // Потеря WebGL (сон телефона, сброс драйвера): пока его нет, виден рисунок
  holst.addEventListener('webglcontextlost', (e) => {
    e.preventDefault();
    poteryan = true;
    gotov = false;
    stop();
    podium.classList.remove('zhivoy');
  });
  holst.addEventListener('webglcontextrestored', () => {
    poteryan = false;
    // Текстура окружения жила в потерянном контексте, её надо снять заново
    scene.environment.dispose();
    try {
      scene.environment = okruzhenie(renderer, n.svet);
    } catch (oshibka) {
      // Контекст вернулся негодным: дальше не рисуем, остаётся рисунок
      slomano = true;
      podium.dataset.bezObema = String(oshibka && oshibka.message ? oshibka.message : oshibka);
      return;
    }
    obnovit();
    pusk();
  });

  obnovit();
  pusk();
}

// Запуск в самом конце: функции и постоянные выше уже определены
const n = JSON.parse(METKA);
pochta(n);
zapusk(n);
