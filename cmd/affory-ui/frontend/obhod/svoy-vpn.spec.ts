import { expect, test } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Раздел «Свой VPN» (план 2026-11-01, Ж5) поверх подставного моста.
//
// Служба здесь заглушка (src/stend/wails-zaglushka.ts): шаги задания идут
// по таймеру, событие svoy-vps-hod приходит тем же кадром, что у службы.
// Что проверяется: мастер проходит от письма хостера до готовых ключей, пароль
// не остаётся на экране, раздел помещается в самое узкое окно, карточка
// проверяет давний сервер сама. Настоящий сервер и SSH живут в приёмке в госте.

const PISMO = [
  "Your server is ready",
  "",
  "IP address: 203.0.113.10",
  "Username: root",
  "Password: obhod-parol-1",
  "SSH port: 22",
].join("\n");

test("установка от письма хостера до готовых ключей", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=svoy-vpn");

  await page.getByTestId("svoy-postavit").click();
  // fill кладёт текст разом, как вставка: окно предлагает стереть буфер.
  await page.getByTestId("svoy-soobshchenie").fill(PISMO);
  await expect(page.getByTestId("svoy-adres")).toHaveValue("203.0.113.10");
  await expect(page.getByTestId("svoy-parol")).toHaveValue("obhod-parol-1");
  await expect(page.getByTestId("svoy-parol")).toHaveAttribute("type", "password");
  await expect(page.getByTestId("svoy-ochistit-bufer")).toBeVisible();

  await page.getByTestId("svoy-dalshe").click();
  await expect(page.getByTestId("svoy-punkty")).toContainText("Ubuntu 24.04");
  await expect(page.getByTestId("svoy-postavit-vpn")).toBeDisabled();
  await page.getByTestId("svoy-pustoy").click();
  await page.getByTestId("svoy-postavit-vpn").click();

  await expect(page.getByTestId("svoy-etapy")).toBeVisible();
  // Заглушка проходит установку за десяток шагов по 0.7 с.
  await expect(page.getByTestId("svoy-gotovo")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("svoy-gotovo")).toContainText("reality");
  await expect(page.getByTestId("svoy-gotovo")).toContainText("hysteria2");
  await expect(page.locator("input[type=password]")).toHaveCount(0);
  expect(await page.locator("body").innerText()).not.toContain("obhod-parol-1");

  await page.getByRole("button", { name: "Готово" }).click();
  const kartochka = page.locator("[data-testid^=svoy-server-]");
  await expect(kartochka).toHaveCount(1);
  await expect(kartochka).toContainText("203.0.113.10");
  await expect(kartochka.getByTestId("svoy-sostoyanie")).toHaveText("работает");

  await nichegoNeSlomalos(page, oshibki);
});

test("в самом узком окне полоса разделов и мастер без горизонтальной прокрутки", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  // 900 это минимальная ширина окна (okno.go).
  await page.setViewportSize({ width: 900, height: 700 });
  await page.goto("/?ekran=svoy-vpn");

  const polosa = page.getByRole("tablist", { name: "Разделы" });
  await expect(polosa.getByRole("tab", { name: /Свой VPN/ })).toHaveAttribute("aria-selected", "true");
  // Каждая вкладка целиком левее кнопок справа. Первая из них GitHub, а
  // последняя вкладка «Настройки»: наехать на кнопки может именно она.
  const knopkiSprava = await page.getByRole("button", { name: "Открыть GitHub Affory", exact: true }).boundingBox();
  expect(knopkiSprava, "кнопка GitHub не нарисована").not.toBeNull();
  const vkladki = await polosa.getByRole("tab").all();
  expect(vkladki).toHaveLength(4);
  for (const v of vkladki) {
    const k = await v.boundingBox();
    expect(k, `вкладка «${await v.innerText()}» не нарисована`).not.toBeNull();
    expect((k?.x ?? 0) + (k?.width ?? 0), `вкладка «${await v.innerText()}» заходит на кнопки окна`).toBeLessThanOrEqual(knopkiSprava?.x ?? 0);
  }

  await page.getByTestId("svoy-postavit").click();
  await expect(page.getByTestId("svoy-soobshchenie")).toBeVisible();
  const shirina = await page.evaluate(() => ({ prokrutka: document.documentElement.scrollWidth, okno: document.documentElement.clientWidth }));
  expect(shirina.prokrutka).toBeLessThanOrEqual(shirina.okno);

  await nichegoNeSlomalos(page, oshibki);
});

test("давний сервер проверяется при открытии, снос без подтверждения не идёт", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=svoy-vpn&svoy=server");

  const kartochka = page.locator("[data-testid^=svoy-server-]");
  await expect(kartochka).toContainText("203.0.113.40");
  // Заглушка ставит проверку двухчасовой давности: раздел проверяет сам.
  await expect(kartochka).toContainText("Проверен только что", { timeout: 15_000 });
  await expect(kartochka.getByTestId("svoy-sostoyanie")).toHaveText("работает");

  await kartochka.getByRole("button", { name: "Действия с сервером" }).click();
  await page.getByRole("menuitem", { name: "Снести с сервера" }).click();
  const vopros = page.getByRole("dialog");
  await expect(vopros).toContainText("Снести VPN с сервера 203.0.113.40?");
  await vopros.getByRole("button", { name: "Отмена" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByText("Сношу VPN с сервера")).toHaveCount(0);

  await nichegoNeSlomalos(page, oshibki);
});

test("сервер не пустил: причина на шаге проверки, путь назад к доступу", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=svoy-vpn");

  await page.getByTestId("svoy-postavit").click();
  await page.getByTestId("svoy-adres").fill("203.0.113.66");
  await page.getByTestId("svoy-parol").fill("nevernyy");
  await page.getByTestId("svoy-dalshe").click();

  await expect(page.getByRole("alert").filter({ hasText: "не принял пароль" })).toBeVisible();
  await page.getByRole("button", { name: "Ввести другой сервер" }).click();
  await expect(page.getByTestId("svoy-adres")).toHaveValue("203.0.113.66");

  await nichegoNeSlomalos(page, oshibki);
});

test("установка, упавшая до запуска окна, возвращает мастер и просит доступ заново", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=svoy-vpn&svoy=ustanovka-otkaz");

  // Мастер вернулся к установке сам: пароль окно не хранит, повтор только
  // через новый ввод доступа.
  await expect(page.getByTestId("svoy-etapy")).toBeVisible();
  await expect(page.getByRole("alert").filter({ hasText: "ни один порт hysteria2 не прошёл" })).toBeVisible({ timeout: 30_000 });
  await page.getByRole("button", { name: "Ввести доступ" }).click();
  await expect(page.getByTestId("svoy-adres")).toHaveValue("203.0.113.40");
  await expect(page.getByTestId("svoy-parol")).toHaveValue("");

  await nichegoNeSlomalos(page, oshibki);
});
