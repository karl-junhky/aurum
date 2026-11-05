import { useEffect, useRef, useState } from "react";
import { slovoPosleChisla } from "../chisla";
import { Karta, Knopka, Segment } from "./ui";
import { Krestik } from "./SpravkaProtokolov";
import { tekstOshibki } from "../ponyatno";

// Выгрузка серверов (26.09.2026): ссылки из exportServers, чтобы перенести
// ключи на другой ПК или телефон строкой или QR.
//
// С 02.10.2026 выгрузка рисуется списком серверов, а не сплошным текстом:
// дюжина ссылок подряд не давала понять, какая из них какой сервер (жалоба
// владельца со скриншотом). У каждой строки свои «Скопировать» и QR, а ключ
// одного сервера открывается и из меню правой кнопки, окном OknoKlyucha.
//
// Текст и код по умолчанию закрыты. Панель открывают и при демонстрации
// экрана, а строка на экране это ключ в трансляции. «Скопировать» работает
// сразу и ничего не показывает.

/** Ключ одного сервера в ответе exportServers. */
export interface KlyuchServera {
  id: string;
  imya: string;
  transport: string;
  /** host:port, IPv6 в скобках. Пусто, когда служба старее окна и строка
   *  восстановлена из текста. */
  adres: string;
  ssylka: string;
}

/** Ответ exportServers, см. cmd/affory-svc/komandy_pachki.go. */
export interface VygruzkaOtvet {
  tekst: string;
  base64: string;
  vsego: number;
  servery: KlyuchServera[];
  propushcheny?: { imya: string; prichina: string }[];
}

type Nazvat = (transport: string) => string;
const kakEst: Nazvat = (t) => t;

/** Подпись под именем: адрес и протокол в том же порядке, что в списке
 *  серверов, чтобы строку выгрузки можно было найти там глазами. */
function podpis(k: KlyuchServera, nazvat: Nazvat): string {
  return [k.adres, nazvat(k.transport)].filter(Boolean).join(" · ");
}

/** Ссылка целиком. Одно нажатие выделяет её всю: ключ копируют строкой, и
 *  выделять его мышью от схемы до имени незачем. */
function SsylkaKlyucha({ ssylka, testId }: { ssylka: string; testId?: string }) {
  return (
    <code
      data-testid={testId}
      className="bg-elevated border-border text-fg-secondary block select-all break-all rounded-md border px-2.5 py-1.5 font-mono text-xs leading-relaxed"
    >
      {ssylka}
    </code>
  );
}

/** Коды QR. Белое поле обязательно: камера ищет тёмные модули на светлом, и
 *  код на тёмной подложке окна не читается. */
export function KodyQr({ kody, imya, shirina, testId }: { kody: string[]; imya: string; shirina: string; testId?: string }) {
  return (
    <div className="flex flex-wrap gap-4" data-testid={testId}>
      {kody.map((kod, i) => (
        <figure key={kod} className={`flex w-full flex-col items-center gap-1.5 ${shirina}`}>
          <img
            src={kod}
            alt={kody.length > 1 ? `QR ${i + 1} из ${kody.length}` : `QR ${imya}`}
            className="aspect-square w-full rounded-md bg-white"
          />
          {kody.length > 1 && (
            <figcaption className="text-fg-muted text-xs">{`код ${i + 1} из ${kody.length}`}</figcaption>
          )}
        </figure>
      ))}
    </div>
  );
}

export function Vygruzka({ vygruzka, zakryt, skopirovat, kodyQr, nazvatTransport = kakEst, zagolovok }: {
  vygruzka: VygruzkaOtvet;
  zakryt: () => void;
  skopirovat?: (tekst: string) => Promise<void>;
  kodyQr?: (tekst: string) => Promise<string[]>;
  nazvatTransport?: Nazvat;
  /** Вместо «Выгрузка: N серверов»: заголовок той кнопки, что её открыла. */
  zagolovok?: string;
}) {
  const [vid, zadatVid] = useState<"spisok" | "base64">("spisok");
  const [klyuchiVidny, zadatKlyuchiVidny] = useState(false);
  const [kody, zadatKody] = useState<string[] | null>(null);
  const [soobshchenie, zadatSoobshchenie] = useState<string | null>(null);
  // Что сделано со строкой. По одной строке за раз: два открытых кода рядом
  // камера читала бы вперемешку, а «скопировано» у двух строк врало бы про
  // одну из них.
  const [skopirovana, zadatSkopirovana] = useState<string | null>(null);
  const [qrStroki, zadatQrStroki] = useState<{ id: string; kody: string[] } | null>(null);
  const [otkazStroki, zadatOtkazStroki] = useState<{ id: string; tekst: string } | null>(null);

  // Новая выгрузка закрывает прежний код: картинка старого списка рядом со
  // свежим текстом врала бы про один из них.
  useEffect(() => {
    zadatKody(null);
    zadatSoobshchenie(null);
    zadatSkopirovana(null);
    zadatQrStroki(null);
    zadatOtkazStroki(null);
  }, [vygruzka]);

  const kopirovatVse = async () => {
    if (!skopirovat) return;
    zadatSkopirovana(null);
    try {
      await skopirovat(vid === "spisok" ? vygruzka.tekst : vygruzka.base64);
      zadatSoobshchenie("скопировано");
    } catch (e: unknown) {
      zadatSoobshchenie(`не скопировалось: ${tekstOshibki(e)}`);
    }
  };
  const qrVseh = async () => {
    if (kody) { zadatKody(null); return; }
    if (!kodyQr) return;
    try {
      // В код всегда идёт список, а не base64: он на треть короче, и чужие
      // клиенты читают оба.
      zadatKody(await kodyQr(vygruzka.tekst));
      zadatQrStroki(null);
      zadatSoobshchenie(null);
    } catch (e: unknown) {
      zadatSoobshchenie(tekstOshibki(e));
    }
  };
  const kopirovatStroku = async (k: KlyuchServera) => {
    if (!skopirovat) return;
    zadatOtkazStroki(null);
    zadatSoobshchenie(null);
    try {
      await skopirovat(k.ssylka);
      zadatSkopirovana(k.id);
    } catch (e: unknown) {
      zadatSkopirovana(null);
      zadatOtkazStroki({ id: k.id, tekst: `не скопировалось: ${tekstOshibki(e)}` });
    }
  };
  const qrStroku = async (k: KlyuchServera) => {
    if (qrStroki?.id === k.id) { zadatQrStroki(null); return; }
    if (!kodyQr) return;
    zadatOtkazStroki(null);
    try {
      zadatQrStroki({ id: k.id, kody: await kodyQr(k.ssylka) });
      zadatKody(null);
    } catch (e: unknown) {
      zadatQrStroki(null);
      zadatOtkazStroki({ id: k.id, tekst: tekstOshibki(e) });
    }
  };

  return (
    <Karta testId="vygruzka">
      <div className="flex flex-col gap-3 px-4 py-3">
        <div className="flex items-center gap-3">
          <span className="text-foreground flex-1 text-sm font-medium" data-testid="vygruzka-itog">
            {zagolovok ?? `Выгрузка: ${vygruzka.vsego} ${slovoPosleChisla(vygruzka.vsego, "сервер", "сервера", "серверов")}`}
          </span>
          <Knopka rang="tekst" testId="vygruzka-zakryt" onClick={zakryt}>Закрыть</Knopka>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Segment<"spisok" | "base64">
            aria-label="вид выгрузки"
            znacheniya={[{ z: "spisok", podpis: "списком" }, { z: "base64", podpis: "одной закодированной строкой" }]}
            vybrano={vid}
            naVybor={(z) => { zadatVid(z); zadatSoobshchenie(null); }}
          />
          <Knopka rang="glavnaya" testId="vygruzka-kopirovat" aktiven={!!skopirovat} onClick={() => void kopirovatVse()}>
            Скопировать всё
          </Knopka>
          {kodyQr && (
            <Knopka rang="vtoraya" testId="vygruzka-qr" onClick={() => void qrVseh()}>
              {kody ? "Скрыть QR" : "QR всех"}
            </Knopka>
          )}
          <Knopka rang="vtoraya" testId="vygruzka-tekst" aria-pressed={klyuchiVidny} onClick={() => zadatKlyuchiVidny(!klyuchiVidny)}>
            {klyuchiVidny ? "Скрыть ключи" : "Показать ключи"}
          </Knopka>
        </div>
        {soobshchenie && (
          <p role="status" className="text-fg-secondary text-xs" data-testid="vygruzka-soobshchenie">{soobshchenie}</p>
        )}
        {kody && <KodyQr kody={kody} imya="всех серверов" shirina="max-w-[360px]" testId="vygruzka-kody" />}
      </div>

      <ul aria-label="ключи по серверам" className="border-border border-t" data-testid="vygruzka-spisok">
        {vygruzka.servery.map((k) => {
          const qrOtkryt = qrStroki?.id === k.id;
          const skop = skopirovana === k.id;
          return (
            <li key={k.id} data-testid={`vygruzka-stroka-${k.id}`} className="border-border flex flex-col gap-2 border-t px-4 py-2 first:border-t-0">
              <div className="flex min-h-8 items-center gap-2">
                <div className="flex min-w-0 flex-1 items-baseline gap-2.5">
                  <span className="text-foreground min-w-0 truncate text-sm font-medium">{k.imya}</span>
                  <span className="text-fg-muted truncate text-xs">{podpis(k, nazvatTransport)}</span>
                </div>
                {skopirovat && (
                  <Knopka
                    rang="tekst"
                    testId={`vygruzka-kopirovat-${k.id}`}
                    aria-label={skop ? `скопировано: ключ ${k.imya}` : `скопировать ключ ${k.imya}`}
                    onClick={() => void kopirovatStroku(k)}
                  >
                    {skop ? "скопировано" : "Скопировать"}
                  </Knopka>
                )}
                {kodyQr && (
                  <Knopka
                    rang="tekst"
                    testId={`vygruzka-qr-${k.id}`}
                    aria-label={qrOtkryt ? `скрыть QR ключа ${k.imya}` : `QR ключа ${k.imya}`}
                    aria-expanded={qrOtkryt}
                    onClick={() => void qrStroku(k)}
                  >
                    {qrOtkryt ? "Скрыть QR" : "QR"}
                  </Knopka>
                )}
              </div>
              {klyuchiVidny && <SsylkaKlyucha ssylka={k.ssylka} testId={`vygruzka-klyuch-${k.id}`} />}
              {qrStroki && qrOtkryt && <KodyQr kody={qrStroki.kody} imya={k.imya} shirina="max-w-[280px]" testId={`vygruzka-kod-${k.id}`} />}
              {otkazStroki?.id === k.id && <p role="status" className="text-warn text-xs">{otkazStroki.tekst}</p>}
            </li>
          );
        })}
      </ul>

      {(vygruzka.propushcheny?.length ?? 0) > 0 && (
        <ul className="text-fg-muted border-border border-t px-4 py-2.5 text-xs" data-testid="vygruzka-propushcheny">
          {vygruzka.propushcheny!.map((p) => (
            <li key={p.imya}>{`не выгружен ${p.imya}: ${p.prichina}`}</li>
          ))}
        </ul>
      )}
    </Karta>
  );
}

/** Окно ключа одного сервера, из меню правой кнопки (02.10.2026).
 *
 *  Открывается уже с ключом и кодом на виду: человек попросил именно
 *  показать, и прятать ключ за второй кнопкой значило бы спросить дважды.
 *  Пока служба не ответила, `klyuch` пуст; отказ приходит строкой `otkaz`. */
export function OknoKlyucha({ imya, klyuch, otkaz, zakryt, skopirovat, kodyQr, nazvatTransport = kakEst }: {
  imya: string;
  klyuch: KlyuchServera | null;
  otkaz: string | null;
  zakryt: () => void;
  skopirovat?: (tekst: string) => Promise<void>;
  kodyQr?: (tekst: string) => Promise<string[]>;
  nazvatTransport?: Nazvat;
}) {
  const okno = useRef<HTMLDivElement>(null);
  const [kody, zadatKody] = useState<string[] | null>(null);
  const [otkazQr, zadatOtkazQr] = useState<string | null>(null);
  const [itogKopii, zadatItogKopii] = useState<string | null>(null);

  // Esc закрывает, фокус уезжает внутрь окна: тот же порядок, что у справки
  // о протоколах.
  useEffect(() => {
    const naKlavishu = (e: KeyboardEvent) => {
      if (e.key === "Escape") zakryt();
    };
    document.addEventListener("keydown", naKlavishu);
    okno.current?.focus();
    return () => document.removeEventListener("keydown", naKlavishu);
  }, [zakryt]);

  // Код рисуется, как только приехал ключ. Ответ, пришедший после закрытия
  // окна или после смены ключа, уже никому не нужен.
  useEffect(() => {
    zadatKody(null);
    zadatOtkazQr(null);
    if (!klyuch || !kodyQr) return;
    let aktualen = true;
    kodyQr(klyuch.ssylka).then(
      (k) => { if (aktualen) zadatKody(k); },
      (e: unknown) => { if (aktualen) zadatOtkazQr(tekstOshibki(e)); },
    );
    return () => { aktualen = false; };
  }, [klyuch, kodyQr]);

  const kopirovat = async () => {
    if (!skopirovat || !klyuch) return;
    try {
      await skopirovat(klyuch.ssylka);
      zadatItogKopii("скопировано");
    } catch (e: unknown) {
      zadatItogKopii(`не скопировалось: ${tekstOshibki(e)}`);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) zakryt();
      }}
    >
      <div
        ref={okno}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label={`Ключ сервера ${imya}`}
        data-testid="klyuch-servera"
        className="border-border bg-surface flex max-h-[85vh] w-full max-w-md flex-col gap-4 overflow-y-auto rounded-xl border p-5 focus-visible:outline-none"
      >
        <header className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h3 className="text-foreground truncate text-base font-semibold">{`Ключ ${imya}`}</h3>
            {klyuch && <p className="text-fg-muted mt-0.5 truncate text-[13px]">{podpis(klyuch, nazvatTransport)}</p>}
          </div>
          <button
            type="button"
            data-testid="klyuch-servera-zakryt"
            aria-label="Закрыть"
            onClick={zakryt}
            className="text-fg-muted hover:text-foreground hover:bg-surface-hover inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md transition-colors"
          >
            <Krestik />
          </button>
        </header>

        {!klyuch && !otkaz && <p role="status" className="text-fg-muted text-[13px]">достаю ключ</p>}
        {otkaz && <p role="alert" className="text-warn text-[13px]" data-testid="klyuch-servera-otkaz">{otkaz}</p>}
        {klyuch && (
          <>
            {kody && <div className="flex justify-center"><KodyQr kody={kody} imya={imya} shirina="max-w-[280px]" testId="klyuch-servera-kod" /></div>}
            {otkazQr && <p className="text-fg-muted text-xs">{otkazQr}</p>}
            <SsylkaKlyucha ssylka={klyuch.ssylka} testId="klyuch-servera-ssylka" />
            {skopirovat && (
              <div className="flex items-center gap-3">
                <Knopka rang="glavnaya" testId="klyuch-servera-kopirovat" onClick={() => void kopirovat()}>Скопировать</Knopka>
                {itogKopii && <span role="status" className="text-fg-secondary text-xs">{itogKopii}</span>}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
