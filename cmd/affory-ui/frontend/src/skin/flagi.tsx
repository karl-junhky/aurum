// Флаги кругом, как в референсе. Эмодзи-флаги Windows не рисует (покажет две
// буквы), поэтому флаг это CSS-градиент: ноль файлов и ноль сети (CSP запрещает
// внешние картинки).
import type { CSSProperties } from "react";

const H = (...c: string[]) => {
  const n = c.length;
  return `linear-gradient(to bottom, ${c.map((x, i) => `${x} ${(i * 100) / n}% ${((i + 1) * 100) / n}%`).join(",")})`;
};
const V = (...c: string[]) => {
  const n = c.length;
  return `linear-gradient(to right, ${c.map((x, i) => `${x} ${(i * 100) / n}% ${((i + 1) * 100) / n}%`).join(",")})`;
};
const KREST = (fon: string, kr: string, x = "36%") =>
  `linear-gradient(${kr},${kr}) center/100% 22% no-repeat, linear-gradient(${kr},${kr}) ${x} 50%/22% 100% no-repeat, ${fon}`;

const FLAGI: Record<string, string> = {
  EE: H("#0072ce", "#111", "#fff"),
  RU: H("#fff", "#0039a6", "#d52b1e"),
  DE: H("#111", "#dd0000", "#ffce00"),
  NL: H("#ae1c28", "#fff", "#21468b"),
  FR: V("#0055a4", "#fff", "#ef4135"),
  IT: V("#009246", "#fff", "#ce2b37"),
  PL: H("#fff", "#dc143c"),
  UA: H("#0057b7", "#ffd700"),
  AT: H("#ed2939", "#fff", "#ed2939"),
  LT: H("#fdb913", "#006a44", "#c1272d"),
  LV: H("#9e3039", "#9e3039", "#fff", "#9e3039", "#9e3039"),
  BG: H("#fff", "#00966e", "#d62612"),
  HU: H("#ce2939", "#fff", "#477050"),
  ES: H("#aa151b", "#f1bf00", "#f1bf00", "#aa151b"),
  SE: KREST("#006aa7", "#fecc00"),
  FI: KREST("#fff", "#003580"),
  NO: KREST("#ba0c2f", "#00205b"),
  DK: KREST("#c8102e", "#fff"),
  CH: "radial-gradient(circle at 50% 50%, #fff 0 0)",
  KZ: "#00afca",
  TR: "radial-gradient(circle at 38% 50%, #fff 0 22%, transparent 23%), #e30a17",
  JP: "radial-gradient(circle at 50% 50%, #bc002d 0 30%, transparent 31%), #fff",
  US: "repeating-linear-gradient(to bottom,#b22234 0 7.7%,#fff 7.7% 15.4%)",
  GB: "linear-gradient(#c8102e,#c8102e) center/100% 18% no-repeat, linear-gradient(#c8102e,#c8102e) center/18% 100% no-repeat, #012169",
  CZ: H("#fff", "#d7141a"),
  RO: V("#002b7f", "#fcd116", "#ce1126"),
  MD: V("#0046ae", "#ffd200", "#cc092f"),
  GE: "#fff",
  AM: H("#d90012", "#0033a0", "#f2a800"),
  CA: V("#d52b1e", "#fff", "#fff", "#d52b1e"),
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

/** Круглый значок: флаг, а если страну не узнали, монограмма на градиенте. */
export function Flag({ imya, razmer = 32 }: { imya: string; razmer?: number }) {
  const kod = stranaPoImeni(imya);
  const stil: CSSProperties = { width: razmer, height: razmer };
  if (kod && FLAGI[kod]) {
    return (
      <span
        aria-hidden
        data-flag={kod}
        className="aurum-flag"
        style={{ ...stil, background: FLAGI[kod] }}
      />
    );
  }
  let h = 0;
  for (const c of imya) h = (h * 31 + c.charCodeAt(0)) % 360;
  return (
    <span
      aria-hidden
      className="aurum-flag aurum-flag-mono"
      style={{ ...stil, background: `linear-gradient(135deg, hsl(${h} 55% 42%), hsl(${(h + 50) % 360} 60% 28%))`, fontSize: razmer * 0.42 }}
    >
      {(imya.trim()[0] ?? "A").toUpperCase()}
    </span>
  );
}
