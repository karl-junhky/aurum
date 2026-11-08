// Страны: узнавание по имени сервера, русское название и столица (для
// меток на планете). Чистый модуль без React, чтобы его проверяли тестами.

export interface Strana { imya: string; lat: number; lon: number }

export const STRANA: Record<string, Strana> = {
  EE: { imya: "Эстония", lat: 59.44, lon: 24.75 }, RU: { imya: "Россия", lat: 55.75, lon: 37.62 },
  DE: { imya: "Германия", lat: 50.11, lon: 8.68 }, NL: { imya: "Нидерланды", lat: 52.37, lon: 4.9 },
  FR: { imya: "Франция", lat: 48.85, lon: 2.35 }, IT: { imya: "Италия", lat: 45.46, lon: 9.19 },
  PL: { imya: "Польша", lat: 52.23, lon: 21.01 }, UA: { imya: "Украина", lat: 50.45, lon: 30.52 },
  AT: { imya: "Австрия", lat: 48.21, lon: 16.37 }, LT: { imya: "Литва", lat: 54.69, lon: 25.28 },
  LV: { imya: "Латвия", lat: 56.95, lon: 24.11 }, BG: { imya: "Болгария", lat: 42.7, lon: 23.32 },
  HU: { imya: "Венгрия", lat: 47.5, lon: 19.04 }, ES: { imya: "Испания", lat: 40.42, lon: -3.7 },
  SE: { imya: "Швеция", lat: 59.33, lon: 18.07 }, FI: { imya: "Финляндия", lat: 60.17, lon: 24.94 },
  NO: { imya: "Норвегия", lat: 59.91, lon: 10.75 }, DK: { imya: "Дания", lat: 55.68, lon: 12.57 },
  CH: { imya: "Швейцария", lat: 47.37, lon: 8.54 }, KZ: { imya: "Казахстан", lat: 51.17, lon: 71.45 },
  TR: { imya: "Турция", lat: 41.01, lon: 28.97 }, JP: { imya: "Япония", lat: 35.68, lon: 139.7 },
  US: { imya: "США", lat: 40.71, lon: -74.0 }, GB: { imya: "Великобритания", lat: 51.5, lon: -0.12 },
  CZ: { imya: "Чехия", lat: 50.08, lon: 14.43 }, RO: { imya: "Румыния", lat: 44.43, lon: 26.1 },
  MD: { imya: "Молдова", lat: 47.01, lon: 28.86 }, GE: { imya: "Грузия", lat: 41.72, lon: 44.79 },
  AM: { imya: "Армения", lat: 40.18, lon: 44.51 }, CA: { imya: "Канада", lat: 43.65, lon: -79.38 },
};

/** Страна из названия сервера: «Estonia 1», «Нидерланды · Амстердам», «NL-02».
 *  Основы слов совпадают с начала слова, двухбуквенные коды только целиком:
 *  `\\b` в JS не знает кириллицы, поэтому границы слов считаются по \\p{L}. */
const STRANY: [string, string[], string[]][] = [
  ["EE", ["estonia", "эстони", "tallinn", "таллин"], ["ee"]],
  ["RU", ["russia", "росси", "moscow", "москв"], ["ru"]],
  ["DE", ["german", "герман", "frankfurt", "франкфурт", "berlin", "берлин"], ["de"]],
  ["NL", ["netherland", "holland", "нидерланд", "голланд", "amsterdam", "амстердам"], ["nl"]],
  ["FR", ["france", "франци", "paris", "париж"], ["fr"]],
  ["IT", ["italy", "итали", "milan", "милан"], ["it"]],
  ["PL", ["poland", "польш", "warsaw", "варшав"], ["pl"]],
  ["UA", ["ukrain", "украин", "kyiv", "киев"], ["ua"]],
  ["AT", ["austria", "австри", "vienna", "вена"], ["at"]],
  ["LT", ["lithuania", "литв", "vilnius", "вильн"], ["lt"]],
  ["LV", ["latvia", "латви", "riga", "рига"], ["lv"]],
  ["BG", ["bulgaria", "болгар", "sofia", "софи"], ["bg"]],
  ["HU", ["hungary", "венгри", "budapest", "будапешт"], ["hu"]],
  ["ES", ["spain", "испани", "madrid", "мадрид"], ["es"]],
  ["SE", ["sweden", "швеци", "stockholm", "стокгольм"], ["se"]],
  ["FI", ["finland", "финлянд", "helsinki", "хельсинки"], ["fi"]],
  ["NO", ["norway", "норвег", "oslo", "осло"], ["no"]],
  ["DK", ["denmark", "дани", "copenhagen", "копенгаген"], ["dk"]],
  ["CH", ["switzerland", "швейцар", "zurich", "цюрих"], ["ch"]],
  ["KZ", ["kazakh", "казахстан", "almaty", "алма"], ["kz"]],
  ["TR", ["turkey", "турци", "istanbul", "стамбул"], ["tr"]],
  ["JP", ["japan", "япони", "tokyo", "токио"], ["jp"]],
  ["US", ["united states", "america", "сша", "америк", "new york", "нью-йорк", "los angeles", "miami"], ["us", "usa"]],
  ["GB", ["britain", "england", "london", "великобритан", "англи", "лондон", "united kingdom"], ["uk", "gb"]],
  ["CZ", ["czech", "чехи", "prague", "прага"], ["cz"]],
  ["RO", ["romania", "румын", "bucharest", "бухарест"], ["ro"]],
  ["MD", ["moldova", "молдав"], ["md"]],
  ["GE", ["georgia", "грузи", "tbilisi", "тбилиси"], ["ge"]],
  ["AM", ["armenia", "армени", "yerevan", "ереван"], ["am"]],
  ["CA", ["canada", "канад", "toronto", "торонто"], ["ca"]],
];

export function stranaPoImeni(imya: string): string | null {
  const t = imya.toLowerCase();
  for (const [kod, osnovy, kody] of STRANY) {
    if (osnovy.some((o) => new RegExp(`(?<![\\p{L}])${o.replace(/[-\s]/g, "[-\\s]")}`, "u").test(t))) return kod;
    if (kody.some((c) => new RegExp(`(?<![\\p{L}])${c}(?![\\p{L}])`, "u").test(t))) return kod;
  }
  return null;
}

