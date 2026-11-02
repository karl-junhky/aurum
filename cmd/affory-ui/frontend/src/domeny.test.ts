import { expect, it } from "vitest";
import { normalizovatDomen, razobratVvodDomenov } from "./domeny";

// C4. Поле сайта принимало ровно одно готовое имя: адрес из адресной строки
// браузера служба отвергала целиком, кириллица не принималась вовсе.

it("адрес из адресной строки превращается в имя сайта", () => {
  for (const vvod of [
    "https://www.example.org/watch?v=1#t",
    "http://www.example.org",
    "www.example.org/",
    " WWW.Example.ORG. ",
  ]) {
    expect(normalizovatDomen(vvod)).toEqual({ domen: "www.example.org" });
  }
});

it("кириллица уезжает в правило тем же punycode, что увидит ядро", () => {
  expect(normalizovatDomen("пример.рф")).toEqual({ domen: "xn--e1afmkfd.xn--p1ai" });
  expect(normalizovatDomen("https://Пример.РФ/страница")).toEqual({ domen: "xn--e1afmkfd.xn--p1ai" });
});

it("адрес не маскируется под имя сайта", () => {
  for (const vvod of ["192.168.1.10", "192.168.0.0/24", "https://10.0.0.1:8443/x", "[2001:db8::1]", "http://[2001:db8::1]/"]) {
    const itog = normalizovatDomen(vvod);
    expect(itog, vvod).toEqual({ prichina: "это адрес, а не имя сайта: правила сайтов работают по имени" });
  }
});

it("негодные строки объясняются каждая своей причиной", () => {
  expect(normalizovatDomen("example")).toEqual({ prichina: "нужно полное имя, например example.com" });
  expect(normalizovatDomen("ftp://example.org")).toEqual({ prichina: "адрес не http и не https" });
  expect(normalizovatDomen("https://user:pass@example.org")).toEqual({ prichina: "адрес с логином и паролем не годится" });
  expect(normalizovatDomen(`${"a".repeat(64)}.example.org`)).toEqual({ prichina: "часть имени длиннее 63 знаков" });
  expect(normalizovatDomen("приме_р.рф")).toEqual({ prichina: "не похоже на имя сайта" });
  // 260 знаков метками по 60: каждая метка годна, имя целиком нет.
  const dlinnoe = Array.from({ length: 5 }, () => "a".repeat(51)).join(".");
  expect(dlinnoe.length).toBeGreaterThan(253);
  expect(normalizovatDomen(dlinnoe)).toEqual({ prichina: "имя длиннее 253 знаков" });
});

it("несколько сайтов за один ввод: любые разделители, порядок и без повторов", () => {
  const { gotovye, otkazy } = razobratVvodDomenov(
    "example.org, https://news.example.org/lenta\nexample.org\n\tпример.рф ; 10.0.0.1  example",
  );
  expect(gotovye.map((g) => g.domen)).toEqual(["example.org", "news.example.org", "xn--e1afmkfd.xn--p1ai"]);
  // Исходный вид сохраняется: punycode рядом с набранным читается, сам по
  // себе нет.
  expect(gotovye[2].ishodnyy).toBe("пример.рф");
  expect(otkazy).toEqual([
    { vvod: "10.0.0.1", prichina: "это адрес, а не имя сайта: правила сайтов работают по имени" },
    { vvod: "example", prichina: "нужно полное имя, например example.com" },
  ]);
});

it("пустой ввод не даёт ни имён, ни жалоб", () => {
  expect(razobratVvodDomenov("   \n  ")).toEqual({ gotovye: [], otkazy: [] });
});
