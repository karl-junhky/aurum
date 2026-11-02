import { expect, test, type Page } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Обход всех экранов и кнопок окна.
//
// Утверждения перенесены из `affory-stend/v-seanse-knopki.ps1` слово в слово
// по смыслу: каждый раздел открывается, на нём есть кнопки, они доступны,
// нажатие не роняет окно и не оставляет экран со словом про отказ. Как и там,
// это НЕ проверка того, что кнопка сделала осмысленную работу: «нажалась и не
// сломалась» и «выполнила команду» разные вещи, и вторая живёт в своих
// судьях.
//
// Что здесь лучше, чем в обходе по дереву доступности:
//
// * кнопка ищется по роли и имени, а не по совпадению строки с регулярным
//   выражением. Прежний список имён после редизайна ловил шесть кнопок из
//   четырёх десятков и считал обход состоявшимся;
// * диалог виден сразу, а не «через полторы секунды, если повезёт». Именно
//   незакрытый диалог дал два ложных ПРОВАЛА из семи прогонов одной сборки;
// * нажимать можно ВСЁ, включая тумблеры. В госте их обходили стороной,
//   потому что «весь трафик через VPN» запирает машину, а «запускать при
//   входе» правит автозапуск гостя. Здесь за тумблером подставной мост.
//
// Чем доказано, что обход не пустой. Не своими зелёными, а МУТАЦИЯМИ
// продукта, 19.09.2026: каждая ставилась по одной и откатывалась после
// прогона.
//
//   | мутация                                   | что покраснело                |
//   |-------------------------------------------|-------------------------------|
//   | `proverka.punkty` → `punkty2`             | окно исчезло после «Проверить»|
//   | «Отмена» диалога удаления теряет обработчик| диалог не закрылся           |
//   | раздел «Правила» теряет свою метку         | раздел не открылся           |
//   | окно зовёт `checkLeaksV2` вместо `checkLeaks` | НЕГОДЕН стенда, не продукта |
//
// Последняя строка и есть то, ради чего затевался переход: вердикт «чинить
// стенд» отличается от «продукт сломан» ТЕКСТОМ, а не догадкой читающего.
//
// Счёт нажатий на 19.09.2026: «Подключение» 9, «Правила» 30, «Настройки» 11.
// Прежний обход по дереву доступности ловил шесть кнопок на всё окно.

/** Разделы полосы. Их четыре: «Серверы» остались вкладкой в коде, но в полосу
 *  после редизайна 16.09.2026 не выходят и открываются кнопкой; «Свой VPN»
 *  добавлен в 1.11.0. */
const RAZDELY = ["Подключение", "Правила", "Свой VPN", "Настройки"] as const;

/** Кнопки окна, а не программы: нажатие закрывает или прячет окно, и обход
 *  после этого судил бы пустоту. */
const NE_NAZHIMAT = ["Закрыть", "Свернуть", "Развернуть", "Открыть GitHub Affory"];

/** Потолок нажатий на раздел. Нажатие раскрывает панели и рождает новые
 *  кнопки, поэтому у обхода в ширину нет своего конца: без потолка цикл
 *  жил бы ровно до срока теста. */
const PREDEL_NAZHATIY = 80;

async function otkrytRazdel(page: Page, imya: string): Promise<void> {
  // Щелчок только когда раздела на экране НЕТ. Лишний щелчок по открытому
  // пересоздаёт экран и сбрасывает его состояние: свёрнутые панели
  // «Настроек» закрывались обратно после каждого нажатия, и обход видел одни
  // и те же шесть кнопок.
  //
  // Судим по метке раздела, а не по выбранной вкладке: «Управлять» уводит на
  // «Серверы», оставляя вкладку «Подключение» выбранной, и проверка по
  // aria-selected сочла бы, что возвращаться некуда.
  const metka = page.locator(`[aria-label="${imya}"]`).first();
  if (!(await metka.isVisible())) await page.getByRole("tab", { name: imya, exact: true }).click();
  // Судим по МЕТКЕ раздела, а не по факту щелчка. Прежний обход трижды подряд
  // обошёл одни и те же кнопки: щелчок уходил в текстовый узел с тем же
  // именем, экран не менялся, а обход считал вкладку открытой.
  await expect(page.locator(`[aria-label="${imya}"]`).first()).toBeVisible();
}

/** Что на экране можно нажать: роль и имя. Тумблер это не кнопка, а
 *  `role="switch"` (Tumbler в ui.tsx), и обход, знавший одни кнопки, не
 *  трогал ни один выключатель настроек. */
interface Element {
  rol: "button" | "switch";
  imya: string;
  /** Номер среди одноимённых. На «Настройках» три РАЗНЫЕ кнопки зовутся
   *  «Проверить» - обновление, утечки и адрес выхода, - и обход по одному
   *  имени нажимал первую трижды, считая остальные пройденными. */
  nomer: number;
}

function klyuch(e: Element): string {
  return `${e.rol}:${e.imya}#${e.nomer}`;
}

/** Видимые и доступные элементы текущего экрана. Имена, а не локаторы: после
 *  нажатия страница перерисовывается, и прежний локатор указывает в никуда. */
async function elementyEkrana(page: Page): Promise<Element[]> {
  const spisok: Element[] = [];
  for (const rol of ["button", "switch"] as const) {
    // Номер считается по ВСЕМ элементам роли, включая скрытые и мёртвые:
    // локатор при нажатии нумерует так же, и счёт только по видимым уехал
    // бы на другую кнопку.
    const vse = await page.getByRole(rol).all();
    const schyot = new Map<string, number>();
    for (const k of vse) {
      const imya = ((await k.getAttribute("aria-label")) ?? (await k.innerText())).trim();
      const nomer = schyot.get(imya) ?? 0;
      schyot.set(imya, nomer + 1);
      if (!(await k.isVisible()) || !(await k.isEnabled())) continue;
      if (!imya || NE_NAZHIMAT.includes(imya)) continue;
      spisok.push({ rol, imya, nomer });
    }
  }
  return spisok;
}

/** Кнопки выхода из диалога, в порядке предпочтения. */
const VYHOD_IZ_DIALOGA = ["Отмена", "Закрыть", "Понятно", "Не сейчас"];

/** Закрывает то, что открылось нажатием: сперва Esc, потом кнопкой выхода.
 *
 *  Escape закрывает не всё. «Удаление программы» его не слушает нарочно -
 *  это единственный необратимый экран, и случайная клавиша не должна им
 *  распоряжаться, - поэтому обход, знавший только Esc, вис на нём и обвинял
 *  окно. Незакрытый диалог ослепляет весь остаток прогона: ровно так рождены
 *  два ложных ПРОВАЛА прежнего обхода по дереву доступности. */
async function zakrytDialog(page: Page): Promise<boolean> {
  const dialog = page.getByRole("dialog");
  if ((await dialog.count()) === 0) return false;

  await page.keyboard.press("Escape");
  if ((await dialog.count()) === 0) return true;

  // Имя снимается ДО нажатия: после закрытия узла нет, и снятие имени само
  // стало бы отказом по сроку вместо внятного сообщения.
  const imyaDialoga = (await dialog.first().getAttribute("aria-label")) ?? "без имени";
  for (const imya of VYHOD_IZ_DIALOGA) {
    const knopka = dialog.first().getByRole("button", { name: imya, exact: true });
    if ((await knopka.count()) > 0 && (await knopka.first().isVisible())) {
      await knopka.first().click();
      break;
    }
  }
  await expect(
    dialog.first(),
    `диалог «${imyaDialoga}» не закрылся ни по Escape, ни кнопкой из ${VYHOD_IZ_DIALOGA.join("/")}`,
  ).toBeHidden({ timeout: 5000 });
  return true;
}

test.describe("обход интерфейса", () => {
  test("полоса несёт ровно четыре раздела и каждый открывается своей меткой", async ({ page }) => {
    const oshibki = lovitOshibki(page);
    await page.goto("/");
    await expect(page.getByTestId("oboloshka")).toBeVisible();

    const zakladki = await page.getByRole("tab").allInnerTexts();
    expect(zakladki.map((s) => s.trim())).toEqual([...RAZDELY]);

    for (const r of RAZDELY) await otkrytRazdel(page, r);
    await nichegoNeSlomalos(page, oshibki);
  });

  test("«Управлять» открывает серверы, которых в полосе нет", async ({ page }) => {
    const oshibki = lovitOshibki(page);
    await page.goto("/");
    await otkrytRazdel(page, "Подключение");
    await page.getByRole("button", { name: "Управлять", exact: true }).click();
    await expect(page.locator('[aria-label="Серверы"]').first()).toBeVisible();
    await nichegoNeSlomalos(page, oshibki);
  });

  for (const razdel of RAZDELY) {
    test(`кнопки раздела «${razdel}» нажимаются и не роняют окно`, async ({ page }) => {
      const oshibki = lovitOshibki(page);
      await page.goto("/");
      await otkrytRazdel(page, razdel);

      // Список пересобирается ПОСЛЕ каждого нажатия, а не однажды в начале.
      //
      // Половина кнопок окна живёт внутри свёрнутых панелей и появляется
      // только после нажатия на заголовок. Обход, собравший список один раз,
      // проходил «Настройки» за секунду и объявлял раздел пройденным, не
      // нажав ни «Проверить», ни «Измерить», ни одной кнопки обновления.
      const nazhato: string[] = [];
      const propushcheno: string[] = [];
      let vsego = 0;
      for (let shag = 0; shag < PREDEL_NAZHATIY; shag++) {
        // Раздел открывается заново перед КАЖДЫМ нажатием: предыдущее могло
        // увести на другой экран, и остаток списка нажимался бы не там.
        await otkrytRazdel(page, razdel);
        const vidno = await elementyEkrana(page);
        vsego = Math.max(vsego, vidno.length);
        // Пустой список это не успех, а молчаливая пустота: ровно так прежний
        // обход «проходил» разделы, на которых после редизайна не узнавал ни
        // одной кнопки.
        expect(vidno.length, `на разделе «${razdel}» не нашлось ни одной кнопки`).toBeGreaterThan(0);

        const el = vidno.find((e) => !nazhato.includes(klyuch(e)) && !propushcheno.includes(klyuch(e)));
        if (!el) break;
        const imya = el.imya;

        const knopka = page.getByRole(el.rol, { name: imya, exact: true }).nth(el.nomer);
        if ((await knopka.count()) === 0 || !(await knopka.isVisible())) {
          propushcheno.push(klyuch(el));
          continue;
        }

        await knopka.click();
        await zakrytDialog(page);

        // Окно живо и раздел на месте. Этого хватает: смысл нажатия судят
        // другие проверки, а здесь вопрос ровно один - не сломалось ли.
        //
        // Имя кнопки стоит В СООБЩЕНИИ, а не в логе рядом. Первый же прогон
        // 19.09.2026 уронил оболочку на разделе «Настройки», и без имени
        // разбор начинался с угадывания, какая из пяти кнопок это сделала.
        await expect(
          page.getByTestId("oboloshka"),
          `окно исчезло после нажатия «${imya}» (нажато до этого: ${nazhato.join(", ") || "ничего"})`,
        ).toBeVisible();
        nazhato.push(klyuch(el));
      }

      // Число в выводе не украшение: молчаливое падение покрытия ловится
      // только им. Обход, собиравший список однажды, проходил «Настройки»
      // за секунду шестью нажатиями, и в отчёте это выглядело успехом.
      console.log(`  «${razdel}»: нажато ${nazhato.length}, пропущено ${propushcheno.length}`);
      expect(nazhato.length, `нажать не удалось ничего из ${vsego}`).toBeGreaterThan(0);

      await nichegoNeSlomalos(page, oshibki);
    });
  }
});
