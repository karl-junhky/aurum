import { Browser, Call, Clipboard, Events, Window } from "@wailsio/runtime";
import { razobratSnimok, razobratOhvat, type ProveritPrilozhenie, type ProveritSoedineniya } from "./ohvat";

export const prochitatOhvatPrilozheniya:ProveritPrilozhenie = async (put) => {
  const response=await zvat("inspectApplication",{put});
  if(response.oshibka)throw new Error(response.oshibka.tekst);
  return razobratOhvat(response.telo);
};

export const prochitatSoedineniya:ProveritSoedineniya = async (filter) => {
  const response=await zvat("listConnections",filter);
  if (response.oshibka) throw new Error(response.oshibka.tekst);
  return razobratSnimok(response.telo);
};

/** Window controls. The title bar is ours (frameless window), so minimise and
 *  close are bridge calls too; Karkas asks App, App asks here. */
export function oknoSvernut(): void {
  void Window.Minimise();
}

export function oknoRazvernut(): void {
  void Window.ToggleMaximise();
}

export function oknoZakryt(): void {
  void Window.Close();
}

// The only file that knows Wails sits underneath. Screens import this and
// nothing from @wailsio/runtime or bindings; granitsa.test.ts enforces it.
// Swapping the shell later means rewriting this file, not every screen.

/** Frame shape mirrors internal/protokol/kadr.go. Kept by hand on purpose:
 *  generated bindings would pin the screens to the shell's type layout. */
export interface Kadr {
  tip: "cmd" | "otvet" | "sobytie";
  id: number;
  imya: string;
  telo?: unknown;
  oshibka?: { kod: string; tekst: string };
}

/** Thrown when the shell itself cannot reach the service: the pipe is closed,
 *  refused or owned by someone else. Distinct from a service-side refusal,
 *  which comes back as a frame with `oshibka`. */
export class KanalNedostupen extends Error {
  constructor(prichina: string) {
    super(prichina);
    this.name = "KanalNedostupen";
  }
}

// Go side: package main, unexported struct `most`, exported method `Zvat`.
// Wails names services by reflect's Type.String(), so "main.most" it is.
const IMYA_ZVAT = "main.most.Zvat";
const SOBYTIE_KANALA = "kanal";
const SOBYTIE_OKNA = "okno";
const SOBYTIE_VKLADKI = "vkladka";

export async function otkrytPapkuZhurnalov(): Promise<void> {
  await Call.ByName("main.most.OtkrytPapkuZhurnalov");
}

/** Настройки даты и времени Windows, кнопка отказа clock-skew (1.10.0). */
export async function otkrytNastroykiVremeni(): Promise<void> {
  await Call.ByName("main.most.OtkrytNastroykiVremeni");
}

/** Выгрузка диагностики (О6 аудита 1.6.1): оболочка спрашивает, куда сохранить,
 *  забирает журналы у службы (без службы читает их сама, 1.10.0) и пишет файл.
 *  Экран получает строку итога, пустая значит «передумал в диалоге». */
export async function sohranitDiagnostiku(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.SohranitDiagnostiku")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Строка в журнал окна (1.10.0): показанный отказ, ошибка страницы. Журнал
 *  это последнее место, куда пишут о сбое, и его собственный отказ некому
 *  показать: он уходит в консоль разработчика и дальше не идёт. */
export function zapisatVZhurnal(tekst: string): void {
  void Call.ByName("main.most.ZapisatVZhurnal", tekst).catch((e: unknown) => {
    console.error("журнал окна не принял запись", e);
  });
}

/** «Affory 1.10.0, Windows 11 (26200)» для строки под отказом. Её отдаёт окно,
 *  а не служба, поэтому строка есть и тогда, когда служба молчит. */
export async function opisanieVersii(): Promise<string> {
  return ((await Call.ByName("main.most.OpisanieVersii")) as string | null) ?? "";
}

/** Sends one command and returns the answer frame. The body travels as JSON
 *  text both ways; typing it is the caller's business. */
export async function zvat(imya: string, telo: unknown = {}): Promise<Kadr> {
  let syroe: string;
  try {
    syroe = await Call.ByName(IMYA_ZVAT, imya, JSON.stringify(telo));
  } catch (e) {
    throw new KanalNedostupen(e instanceof Error ? e.message : String(e));
  }
  return JSON.parse(syroe) as Kadr;
}

/** First run: is AfforySvc registered with the service manager at all. */
export async function sluzhbaUstanovlena(): Promise<boolean> {
  return (await Call.ByName("main.most.SluzhbaUstanovlena")) as boolean;
}

/** Launches `affory-svc install` elevated (the one UAC prompt). Resolves as
 *  soon as the prompt is answered; rejects when it is declined or the
 *  service binary is missing next to the program. */
export async function ustanovitSluzhbu(): Promise<void> {
  try {
    await Call.ByName("main.most.UstanovitSluzhbu");
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Uninstall, elevated, with the keys question already answered. The shell
 *  quits right after the launch; nothing to await beyond the UAC answer. */
export async function udalitProgrammu(steretKlyuchi: boolean): Promise<void> {
  try {
    await Call.ByName("main.most.UdalitProgrammu", steretKlyuchi);
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Перезапуск ОКНА с повышением прав. Служба смотрит на токен того, кто пришёл
 *  в канал, поэтому повысить одну команду нельзя: повышается всё окно. Старое
 *  закрывается само, через мгновение после запроса прав. Отказ от запроса это
 *  отклонённое обещание, а не поломка. */
export async function perezapustitSPravami(vkladka: string): Promise<void> {
  try {
    await Call.ByName("main.most.PerezapustitSPravami", vkladka);
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Вкладка, с которой окно просили открыться: её передаёт себе же окно,
 *  поднимаясь с правами. Пустая строка значит обычный запуск. */
export async function startovayaVkladka(): Promise<string> {
  try {
    return (await Call.ByName("main.most.StartovayaVkladka")) as string;
  } catch {
    // Старая оболочка метода не знает. Обычный запуск это не ошибка.
    return "";
  }
}

/** Перезапуск окна ПОСЛЕ обновления, без запроса прав. Подмена меняет файл на
 *  диске, а работающий процесс держит прежний образ и рисует старый интерфейс:
 *  13.09.2026 в госте служба после подмены назвалась 1.1.2 при окне со сборкой
 *  0.9.9. Новое окно поднимается из каталога программы, это уходит само. */
export async function perezapustitOkno(): Promise<void> {
  try {
    await Call.ByName("main.most.PerezapustitOkno");
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Native file dialog for an update archive; "" when nothing was chosen. */
export async function vybratArhiv(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.VybratArhiv")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Файл ключа SSH для своего сервера: имя и содержимое в base64. */
export interface FaylKlyucha {
  imya: string;
  base64: string;
}

/** Выбор и чтение закрытого ключа SSH (1.11.0). Файл читает оболочка, служба
 *  получает только содержимое. null значит «закрыл диалог», сбой бросает. */
export async function vybratKlyuchSsh(): Promise<FaylKlyucha | null> {
  let otvet: unknown;
  try {
    otvet = await Call.ByName("main.most.VybratKlyuchSsh");
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
  if (otvet == null) return null;
  const o = otvet as { imya?: unknown; base64?: unknown };
  if (typeof o.imya !== "string" || typeof o.base64 !== "string" || o.base64 === "") {
    throw new Error("окно выбора ключа вернуло не файл");
  }
  return { imya: o.imya, base64: o.base64 };
}

/** One running process of this session, for the rules form. */
export interface Zapushchennyy {
  imya: string;
  put: string;
}

/** Processes of the person's session, one per exe, sorted by name. The shell
 *  answers, not the service: session 0 sees svchost, not the games. */
export async function spisokProtsessov(): Promise<Zapushchennyy[]> {
  return ((await Call.ByName("main.most.SpisokProtsessov")) ?? []) as Zapushchennyy[];
}

/** The dialog returns a path, not permission to launch its unsuspecting executable. */
export async function vybratPrilozhenie(): Promise<string> {
  const path: unknown = await Call.ByName("main.most.VybratPrilozhenie");
  if (path == null) return "";
  if (typeof path !== "string") throw new Error("окно выбора вернуло не путь");
  return path;
}

/** Subscribes to service events. Returns the unsubscribe function. */
export function naSobytie(obrabotchik: (kadr: Kadr) => void): () => void {
  return Events.On(SOBYTIE_KANALA, (sobytie: { data: unknown }) => {
    // Data arrives as the JSON string the Go side emitted; anything else is
    // a shell bug, and swallowing it would hide exactly that.
    if (typeof sobytie.data !== "string") return;
    obrabotchik(JSON.parse(sobytie.data) as Kadr);
  });
}

/** Видимость окна: Go сообщает «показалось» и «спряталось» отсюда, а не Wails
 *  своими WindowShow и WindowHide. Причина записана в main.go: при ПЕРВОМ
 *  показе родное событие не приходит вовсе, а окно у нас ещё и прячется
 *  крестиком вместо закрытия, чего родные события не различают.
 *
 *  Возвращает функцию отписки. */
export function naVidimostOkna(obrabotchik: (vidno: boolean) => void): () => void {
  return Events.On(SOBYTIE_OKNA, (sobytie: { data: unknown }) => {
    if (typeof sobytie.data !== "boolean") return;
    obrabotchik(sobytie.data);
  });
}

/** Трей просит открыть названную вкладку: его пункт «Обновить до X» обязан
 *  привести человека туда, где обновляются, а не просто показать окно на
 *  брошенной вкладке.
 *
 *  Возвращает функцию отписки. */
export function naVkladku(obrabotchik: (vkladka: string) => void): () => void {
  return Events.On(SOBYTIE_VKLADKI, (sobytie: { data: unknown }) => {
    if (typeof sobytie.data !== "string") return;
    obrabotchik(sobytie.data);
  });
}

/** Clipboard text for the "из буфера" button; "" means EMPTY and nothing
 *  else. An unreadable clipboard throws: turning it into "" made the screen
 *  say "буфер обмена пуст" about a clipboard it never managed to read.
 *  The text never goes back to the screen: it is judged and sent, or refused. */
export async function tekstBufera(): Promise<string> {
  try {
    return (await Clipboard.Text()) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** QR с экрана, шаг первый: оболочка прячет окно, снимает экраны и отдаёт
 *  сводку найденного без секретов. Ссылка остаётся в оболочке. Форму сводки
 *  проверяет экран (svodkaQrIz в Servery.tsx). */
export async function naytiQrNaEkrane(): Promise<unknown> {
  try {
    return await Call.ByName("main.most.NaytiQrNaEkrane");
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Шаг второй, по нажатию: оболочка отправляет найденное в службу и отдаёт
 *  готовую строку итога. */
export async function dobavitNaydennoeQr(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.DobavitNaydennoeQr")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Отмена: оболочка забывает найденное. Отказ здесь не важен человеку, но
 *  молча его не глотаем. */
export function zabytQr(): void {
  Call.ByName("main.most.ZabytQr").catch((e: unknown) => {
    console.warn("QR не забыт:", e instanceof Error ? e.message : String(e));
  });
}

/** Выгрузка серверов: оболочка рисует QR в памяти и отдаёт PNG data URI, по
 *  одному на код. Текст с ключами никуда не пишется. */
export async function kodyQr(tekst: string): Promise<string[]> {
  try {
    const kody = await Call.ByName("main.most.KodyQr", tekst);
    return Array.isArray(kody) ? kody.filter((k): k is string => typeof k === "string") : [];
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Кладёт текст в буфер обмена. */
export async function skopirovatTekst(tekst: string): Promise<void> {
  try {
    await Clipboard.SetText(tekst);
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Native "save as" dialog for the profile file; "" means the person closed
 *  it, and anything else that goes wrong throws. */
export async function vybratKudaSohranit(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.VybratKudaSohranit")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Native open dialog for the profile file; "" means the person closed it. */
export async function vybratOtkuda(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.VybratOtkuda")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Writes what exportProfile answered. The body travels as base64 because a
 *  frame body is JSON; the file on disk is the raw bytes, byte for byte the
 *  same as `affory-cli profile export` writes.
 *
 *  The profile PASSWORD never comes anywhere near this call: it goes to the
 *  service inside the command body and nowhere else. */
export async function sohranitProfil(put: string, profil: string): Promise<void> {
  try {
    await Call.ByName("main.most.SohranitProfil", put, profil);
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Reads a profile file for importProfile and returns it base64-encoded. */
export async function prochitatProfil(put: string): Promise<string> {
  try {
    return ((await Call.ByName("main.most.ProchitatProfil", put)) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** A fixed project URL, because the browser is not a command interpreter. */
export function otkrytGitHub(): Promise<void> {
  return Browser.OpenURL("https://github.com/zxczxczxczxczxczxc1111/affory");
}
