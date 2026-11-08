// @vitest-environment node
//
// Сторож моста AURUM. Пилюля читает экраны Affory по data-testid и aria-label
// и нажимает их кнопки (dom.ts). Если новая версия апстрима переименует хоть
// один якорь, пилюля молча перестанет работать. Этот тест падает вместо неё,
// и ночная синхронизация не вольёт такое изменение в main.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const ekrany = (f: string) => readFileSync(fileURLToPath(new URL(`../ekrany/${f}`, import.meta.url)), "utf8");

const YAKORYA: [string, string][] = [
  ["Glavnyy.tsx", 'data-testid="glavnoe-deystvie"'],
  ["Glavnyy.tsx", 'data-testid="nesushchiy"'],
  ["Glavnyy.tsx", 'data-testid="v-seti"'],
  ["Glavnyy.tsx", 'data-testid="zaderzhka"'],
  ["Glavnyy.tsx", 'testId="skorost-priema"'],
  ["Glavnyy.tsx", 'testId="skorost-otdachi"'],
  ["Glavnyy.tsx", 'aria-label="Подключение"'],
  ["Glavnyy.tsx", 'aria-label="Куда идёт трафик"'],
  ["Glavnyy.tsx", "affory-server-name"],
  ["Glavnyy.tsx", "affory-protocol"],
  ["KatalogServerov.tsx", 'aria-label="Список серверов"'],
  ["KatalogServerov.tsx", "`Подключиться к ${s.imya}`"],
  ["Glavnyy.tsx", "Проверить серверы"],
  ["Glavnyy.tsx", "Проверяю отклик"],
  ["Glavnyy.tsx", "Добавить сервер"],
  ["KatalogServerov.tsx", "`Обновить ${g.imya}`"],
  ["Karkas.tsx", 'role="tab"'],
  ["Karkas.tsx", "af-brand"],
  ["vkladki.ts", "Подключение"],
  ["vkladki.ts", "Настройки"],
];

describe("мост AURUM держится за якоря Affory", () => {
  it.each(YAKORYA)("%s содержит %s", (fayl, yakor) => {
    expect(ekrany(fayl)).toContain(yakor);
  });
});
