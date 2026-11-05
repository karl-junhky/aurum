// Собирает танцующего кота в один файл ../vshito/kot.js: three, модуль кота,
// танцы и сцена, без неиспользуемых частей three и со сжатыми именами.
// Файл вшивается в пакет zaglushka, поэтому после правки kot.js, tancy.js или
// stsena.js сборку надо повторить и прогнать go test ./zaglushka/.
//
// Запуск из этого каталога:
//   npm install && node sobrat.mjs
// Без установки можно указать уже стоящие пакеты тех же версий:
//   ZAGLUSHKA_THREE=<каталог пакета three> ZAGLUSHKA_ESBUILD=<каталог пакета esbuild> node sobrat.mjs
// Версии сверяются с package.json: другая версия дала бы другие байты.
//
// kot.js и tancy.js это копии общих модулей кота из сайта-подарка «DJ МяуBASS»,
// здесь их не правим, чтобы сравнение с исходником оставалось простым.
import { readFileSync, writeFileSync, renameSync, rmSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const zdes = dirname(fileURLToPath(import.meta.url));
const vyhod = join(zdes, '..', 'vshito', 'kot.js');
const METKA = '__NASTROYKA_KOTA__';
// Тот же список проверяет тест пакета; здесь он ловит ошибку до записи файла
const ZAPRET = /affory|vpn|впн|proxy|прокси|reality|sing-?box|xray|vless|vmess|hysteria|trojan|shadowsocks|tunnel|туннел|обход блок/i;

const nuzhno = JSON.parse(readFileSync(join(zdes, 'package.json'), 'utf8')).devDependencies;

function paket(imya, peremennaya) {
  const dir = process.env[peremennaya] || join(zdes, 'node_modules', imya);
  let versiya;
  try {
    versiya = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8')).version;
  } catch (e) {
    throw new Error(`нет пакета ${imya} в ${dir}: npm install или ${peremennaya} (${e.message})`);
  }
  if (versiya !== nuzhno[imya]) throw new Error(`${imya} ${versiya} в ${dir}, а нужна ${nuzhno[imya]}`);
  return dir;
}

// Сайт европейский: русские сообщения об ошибках модуля кота заменяются при
// сборке, сам kot.js остаётся копией исходника. Каждая замена обязана
// сработать, иначе исходник поменялся и список надо обновить
const PEREVOD = [
  ['форма кота вылезла за рамку сетки у', 'shape left the grid at'],
  ['three поменял шейдер: окрас кота не встал на место', 'shader patch failed: body colour not applied'],
  ['у кота нет кости', 'no such bone:'],
];

const three = join(paket('three', 'ZAGLUSHKA_THREE'), 'build', 'three.module.js');
const esbuild = createRequire(import.meta.url)(paket('esbuild', 'ZAGLUSHKA_ESBUILD'));

const itog = await esbuild.build({
  entryPoints: [join(zdes, 'stsena.js')],
  bundle: true,
  minify: true,
  format: 'esm',
  charset: 'utf8',
  legalComments: 'eof',
  write: false,
  logLevel: 'warning',
  plugins: [{
    name: 'three',
    setup(b) {
      b.onResolve({ filter: /^three$/ }, () => ({ path: three }));
    },
  }, {
    name: 'perevod',
    setup(b) {
      b.onLoad({ filter: /[\\/]kot\.js$/ }, (a) => {
        let tekst = readFileSync(a.path, 'utf8');
        for (const [ru, en] of PEREVOD) {
          if (!tekst.includes(ru)) throw new Error(`в kot.js нет строки «${ru}»: обнови PEREVOD`);
          tekst = tekst.split(ru).join(en);
        }
        return { contents: tekst, loader: 'js' };
      });
    },
  }],
});

const kod = itog.outputFiles[0].text.replace(/\r\n/g, '\n');
const metok = kod.split(METKA).length - 1;
if (metok !== 1) throw new Error(`метка настроек встречается ${metok} раз, нужна ровно одна`);
const zapret = kod.match(ZAPRET);
if (zapret) throw new Error(`в собранном коде запрещённое слово: ${zapret[0]}`);
const kirillica = kod.match(/.{0,30}[Ѐ-ӿ].{0,30}/);
if (kirillica) throw new Error(`в собранном коде осталась кириллица: ${kirillica[0]}`);
const adresa = [...kod.matchAll(/https?:\/\/[^\s"'`)]+/g)].map((m) => m[0]).filter((a) => a !== 'http://www.w3.org/1999/xhtml');
if (adresa.length) throw new Error(`в собранном коде внешние адреса: ${adresa.join(', ')}`);

// Через временный файл: оборванная запись не оставит полфайла под рабочим именем
try {
  writeFileSync(`${vyhod}.tmp`, kod, 'utf8');
  renameSync(`${vyhod}.tmp`, vyhod);
} catch (e) {
  // Временный файл не оставляем рядом с рабочим, саму ошибку отдаём наверх
  rmSync(`${vyhod}.tmp`, { force: true });
  throw e;
}
console.log(`${vyhod}: ${Buffer.byteLength(kod)} байт`);
