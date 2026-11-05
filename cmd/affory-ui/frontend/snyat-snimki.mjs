// Снимки README: те же экраны, что уезжают в выпуск, но с показательными
// данными из заглушки стенда (vite.stend.config.ts).
//
// Запуск: сначала `npx vite --config vite.stend.config.ts`, затем
// `node snyat-snimki.mjs`. Своего браузера Playwright не тянет, берётся
// пользовательский Chrome через channel.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

const BAZA = "http://127.0.0.1:9245";
const KUDA = fileURLToPath(new URL("../../../assets/screenshots/", import.meta.url));

// 1180x860 при масштабе 1.5 даёт 1770x1290: размер прежних снимков README, и
// менять его нельзя без пересъёмки всех сразу, иначе в таблице поедет вёрстка.
const OKNO = { width: 1180, height: 860 };
const MASHTAB = 1.5;

// Письмо хостера для кадра мастера. Пароль в окне скрыт точками.
const PISMO = ["Сервер готов", "", "IP-адрес: 203.0.113.40", "Логин: root", "Пароль: demo-parol-1"].join("\n");

// Вкладки правил выбираются нажатием, а не адресом: подпись под снимком в
// README обещает конкретную вкладку, и кадр обязан показывать именно её.
// Прежний снимок показывал вкладку сервисов при подписи «Правила приложений».
//
// Снимков стало семь (21.09.2026, по просьбе владельца «скрины всех экранов»):
// до этого README показывал три из шести экранов, и «Управлять» - то место,
// куда человек идёт первым делом, - не был виден вовсе.
// С 02.10.2026 все кадры одной высоты: README ставит их парами в таблицу, и
// кадры разной высоты давали рваные ряды.
const KADRY = [
  // Замеры нажимаются и на главном: без них у каждого сервера «Не измерен», а
  // это половина смысла экрана.
  { imya: "connection.jpg", ekran: "podklyuchenie", knopka: "Проверить серверы" },
  { imya: "servers.jpg", ekran: "servery", otkryt: "zamerit-zaderzhki" },
  { imya: "services.jpg", ekran: "pravila", nazhat: "Сервисы" },
  { imya: "applications.jpg", ekran: "pravila", nazhat: "Приложения" },
  { imya: "sites.jpg", ekran: "pravila", nazhat: "Сайты" },
  { imya: "settings.jpg", ekran: "nastroyki" },
  // Справка это окно поверх экрана подключения, и открывается оно значком
  // вопроса у заголовка «Серверы».
  { imya: "protocols.jpg", ekran: "podklyuchenie", knopka: "Проверить серверы", otkryt: "spravka-protokolov-otkryt", obrezat: "spravka-protokolov" },
  // «Свой VPN» (06.11.2026): путь от письма хостера до карточки сервера.
  // Ход установки в заглушке идёт по таймеру, поэтому кадр ждёт свою строку,
  // а не время, и снимается сразу: общая пауза унесла бы установку дальше.
  {
    imya: "svoy-vpn-pismo.jpg", ekran: "svoy-vpn",
    zhdat: async (page) => {
      await page.getByTestId("svoy-postavit").click();
      await page.getByTestId("svoy-soobshchenie").fill(PISMO);
      await page.getByTestId("svoy-adres").waitFor();
    },
  },
  {
    imya: "svoy-vpn-porty.jpg", ekran: "svoy-vpn", dop: "&svoy=ustanovka", srazu: true,
    zhdat: (page) => page.getByText("Пробую 5443 по UDP").waitFor(),
  },
  {
    imya: "svoy-vpn-gotovo.jpg", ekran: "svoy-vpn", dop: "&svoy=ustanovka&domen=est", srazu: true,
    zhdat: (page) => page.getByTestId("svoy-gotovo").waitFor({ timeout: 30_000 }),
  },
  // Карточка при открытии проверяет сервер сама: кадр ждёт конца проверки.
  // Строка «Свободно на сервере» есть только у законченной проверки, а
  // ожидание скрытия «Проверяю сервер» сработало бы и до её начала.
  {
    imya: "svoy-vpn-server.jpg", ekran: "svoy-vpn", dop: "&svoy=server&domen=est",
    zhdat: async (page) => {
      await page.getByText("Свободно на сервере").waitFor({ timeout: 30_000 });
      await page.getByText("Проверяю сервер").waitFor({ state: "hidden", timeout: 30_000 });
    },
  },
];

mkdirSync(KUDA, { recursive: true });

const browser = await chromium.launch({ channel: "chrome" });
const context = await browser.newContext({ viewport: OKNO, deviceScaleFactor: MASHTAB });
const page = await context.newPage();

for (const kadr of KADRY) {
  await page.setViewportSize(OKNO);
  await page.goto(`${BAZA}/?ekran=${kadr.ekran}${kadr.dop ?? ""}`, { waitUntil: "networkidle" });
  if (kadr.zhdat) await kadr.zhdat(page);
  if (kadr.knopka) {
    await page.getByRole("button", { name: kadr.knopka }).click();
    await page.waitForTimeout(600);
  }
  if (kadr.nazhat) {
    // По роли, а не по тексту: подпись вкладки лежит в одном узле со счётчиком
    // правил, и точный поиск текста её не находит вовсе.
    await page.getByRole("tab", { name: kadr.nazhat }).click();
    await page.waitForTimeout(400);
  }
  if (kadr.otkryt) {
    await page.getByTestId(kadr.otkryt).click();
    await page.waitForTimeout(400);
  }
  // Экран настроек длиннее окна и после перехода приезжает ПРОКРУЧЕННЫМ:
  // снимок начинается с середины, шапки нет. Одного window.scrollTo мало,
  // прокручен внутренний узел, поэтому гасятся все сразу.
  await page.evaluate(() => {
    window.scrollTo(0, 0);
    document.querySelectorAll("*").forEach((el) => {
      if (el.scrollTop) el.scrollTop = 0;
    });
  });
  if (kadr.obrezat) {
    // Окно со своей прокруткой режется краем экрана посреди абзаца, и кадр
    // читается как недоснятый. Нижний край окна подводится к концу последнего
    // протокола, который виден целиком. Тексты при этом не меняются, только
    // высота окна на снимке.
    await page.evaluate((testId) => {
      const okno = document.querySelector(`[data-testid="${testId}"]`);
      if (!okno) return;
      const verh = okno.getBoundingClientRect().top;
      const niz = okno.getBoundingClientRect().bottom - 20;
      let granica = 0;
      okno.querySelectorAll("dl > div").forEach((blok) => {
        const r = blok.getBoundingClientRect();
        if (r.bottom <= niz) granica = Math.max(granica, r.bottom);
      });
      // 10 px под текстом: черта следующего протокола стоит на 12 px ниже, и
      // запас больше этого выводит её полоской по нижнему краю окна.
      if (granica > 0) okno.style.maxHeight = `${Math.ceil(granica - verh + 10)}px`;
    }, kadr.obrezat);
  }
  // Сфера на главном экране анимирована, а замеры и скорость приезжают
  // отдельными кадрами: скорость считается по двум снимкам статистики.
  if (!kadr.srazu) await page.waitForTimeout(2200);
  await page.screenshot({ path: KUDA + kadr.imya, type: "jpeg", quality: 92 });
  console.log(`снят ${kadr.imya} (${kadr.ekran})`);
}

await browser.close();
