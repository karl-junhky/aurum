// @vitest-environment node
//
// Один голос на всё окно. Разбор 13.09.2026 показал два: главный экран,
// маршруты и замер обращались на «вы» с точками в конце («Нажмите на сферу»,
// «Ваши отдельные правила»), а серверы, настройки и весь словарь отказов на
// «ты» без точек. Человек читает одно окно, а не пять авторов.
//
// Проверка файловая, как granitsa.test.ts: глазами такой разнобой не ловится,
// он расползается по новым экранам быстрее, чем его замечают на ревизии.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

function vseFayly(kat: string): string[] {
  const itog: string[] = [];
  for (const imya of readdirSync(kat)) {
    const put = join(kat, imya);
    if (statSync(put).isDirectory()) itog.push(...vseFayly(put));
    else if (/\.tsx?$/.test(imya) && !/\.test\.tsx?$/.test(imya)) itog.push(put);
  }
  return itog;
}

/** Комментарии тут ни при чём: они написаны для того, кто читает код. */
function bezKommentariev(tekst: string): string {
  return tekst.replace(/\/\*[\s\S]*?\*\//g, " ").replace(/\/\/[^\n]*/g, " ");
}

// Границы руками, а не \b: в JS она считается по \w, то есть по латинице, и
// на кириллице молча не совпадает ни разу. Сторож с \b был бы зелёным всегда,
// а разнобой голосов так и остался бы в окне (грабля поймана 13.09.2026).
const L = "а-яёА-ЯЁ";
const NA_VY = new RegExp(`(?<![${L}])(вы|вам|вас|ваш[${L}]*|[${L}]{3,}?(ите|йте|ьте))(?![${L}])`, "i");
const MY = new RegExp(`(?<![${L}])(мы|наш[${L}]*|[${L}]{2,}?(аем|яем|ряем))(?![${L}])`, "i");

describe("один голос на всё окно", () => {
  const koren = fileURLToPath(new URL(".", import.meta.url));
  const fayly = vseFayly(koren);

  it("файлов набралось, иначе проверять нечего", () => {
    expect(fayly.length).toBeGreaterThan(10);
  });

  it.each(fayly)("%s говорит на «ты», а не на «вы»", (put) => {
    const stroki = bezKommentariev(readFileSync(put, "utf8")).split("\n");
    const nayden = stroki
      .map((s, i) => ({ s: s.trim(), i: i + 1 }))
      .filter(({ s }) => NA_VY.test(s));
    expect(nayden.map(({ s, i }) => `${i}: ${s}`)).toEqual([]);
  });

  // Словарь. «Туннель» и «VPN» жили в окне вперемешку, и одна строка главного
  // экрана ухитрялась совместить оба: «VPN через туннель, узел при выключенном
  // VPN». Человеку показывается VPN, «туннель» остаётся словом кода и
  // комментариев.
  it.each(fayly)("%s зовёт соединение VPN, а не туннелем", (put) => {
    const stroki = bezKommentariev(readFileSync(put, "utf8")).split("\n");
    const nayden = stroki
      .map((s, i) => ({ s: s.trim(), i: i + 1 }))
      .filter(({ s }) => /туннел/i.test(s));
    expect(nayden.map(({ s, i }) => `${i}: ${s}`)).toEqual([]);
  });

  // «DNS» человеку ничего не говорит (И6, 02.11.2026): окно называет это
  // поиском адреса сайта. Остаются только имена, которые человек увидит сам:
  // настройка браузера «безопасный DNS» и название списков HaGeZi.
  it.each(fayly)("%s не говорит «DNS», кроме имён настроек и списков", (put) => {
    const stroki = bezKommentariev(readFileSync(put, "utf8")).split("\n");
    const nayden = stroki
      .map((s, i) => ({ s: s.trim(), i: i + 1 }))
      .filter(({ s }) => /DNS/.test(s.replace(/[Бб]езопасн[а-яё]* DNS|HaGeZi DNS Blocklists/g, "")));
    expect(nayden.map(({ s, i }) => `${i}: ${s}`)).toEqual([]);
  });

  it.each(fayly)("%s не говорит от «мы»", (put) => {
    const stroki = bezKommentariev(readFileSync(put, "utf8")).split("\n");
    const nayden = stroki
      .map((s, i) => ({ s: s.trim(), i: i + 1 }))
      .filter(({ s }) => MY.test(s));
    expect(nayden.map(({ s, i }) => `${i}: ${s}`)).toEqual([]);
  });
});
