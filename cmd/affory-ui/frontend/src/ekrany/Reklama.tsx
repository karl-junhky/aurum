import { useRef, useState } from "react";
import type { StatusOtvet } from "../protokol";
import { PREDEL_ISKLYUCHENIY, UROVNI, strokiSostoyaniya, type ReklamaPravila, type UrovenReklamy } from "../reklama";
import { razobratVvodDomenov } from "../domeny";
import { slovoPosleChisla } from "../chisla";
import { IkSayt } from "../ikonki";
import { Knopka, Pole, Segment, Svorachivaemyy, Tumbler } from "./ui";

// Вкладка «Реклама» раздела правил (28.09.2026). Настройка едет общим
// черновиком: переключатель, уровень и исключения ничего не шлют сами, всё
// применяется одной кнопкой вместе с остальными правилами.

/** Сколько разобранных имён показывать в предпросмотре, как у сайтов. */
const POKAZAT_V_PREDPROSMOTRE = 8;

export function Reklama({ reklama, sohranyonnoe, status, disabled, naSmenu, soobshchit }: {
  /** Настройка с черновиком: то, что человек видит и правит. */
  reklama: ReklamaPravila;
  /** Настройка, какой её прислала служба: строки состояния считаются по ней. */
  sohranyonnoe: ReklamaPravila;
  status: StatusOtvet;
  disabled: boolean;
  naSmenu: (next: ReklamaPravila) => boolean;
  soobshchit: (tekst: string) => void;
}) {
  const [vvod, setVvod] = useState("");
  const [udalit, setUdalit] = useState<string | null>(null);
  const pole = useRef<HTMLInputElement | null>(null);
  const razbor = razobratVvodDomenov(vvod);
  const stroki = strokiSostoyaniya(sohranyonnoe, status.reklama, status.sostoyanie);
  const uroven = UROVNI.find((u) => u.z === reklama.uroven);

  const dobavit = () => {
    if (disabled || razbor.gotovye.length === 0) return;
    const novye = razbor.gotovye.map((g) => g.domen).filter((d) => !reklama.razresheno.includes(d));
    if (novye.length === 0) {
      setVvod("");
      soobshchit("Такое исключение уже есть в списке.");
      return;
    }
    const itog = [...reklama.razresheno, ...novye];
    // Предел службы: весь набор отвергается целиком, поэтому упереться в него
    // лучше здесь, чем получить отказ на применение.
    if (itog.length > PREDEL_ISKLYUCHENIY) {
      soobshchit(`Исключений может быть не больше ${PREDEL_ISKLYUCHENIY}: сейчас ${reklama.razresheno.length}, а в этом вводе ещё ${novye.length}.`);
      return;
    }
    naSmenu({ ...reklama, razresheno: itog });
    setVvod("");
    soobshchit(
      novye.length > 1
        ? `${novye.length} ${slovoPosleChisla(novye.length, "исключение", "исключения", "исключений")} добавлено в черновик. Нажми «Применить изменения», когда закончишь редактирование.`
        : "Исключение добавлено в черновик. Нажми «Применить изменения», когда закончишь редактирование.",
    );
  };

  return (
    <div className="flex flex-col gap-5">
      {/* Под блокировкой сети вне VPN переключатель НЕ гаснет, в отличие от
          российского списка: блок стоит в конфиге ядра во всех режимах. */}
      <div className="border-border bg-surface flex items-center gap-4 rounded-xl border px-4 py-3" aria-label="Реклама и трекеры">
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-foreground text-sm font-medium">Блокировать рекламу и трекеры</span>
          <span className="text-fg-muted text-[13px] leading-relaxed">
            Адреса из списка не открываются ни через VPN, ни напрямую. Работает, пока VPN подключён, во всех
            режимах, включая защиту сети
          </span>
        </span>
        <Tumbler
          testId="reklama-vkl"
          podpis="Блокировать рекламу и трекеры"
          aktiven={!disabled}
          vkl={reklama.vkl}
          naSmenu={(v) => naSmenu({ ...reklama, vkl: v })}
        />
      </div>

      {/* Пустой блок в колонке с gap дал бы лишний отступ. */}
      {stroki.length > 0 && (
        <div className="flex flex-col gap-1 text-[13px] leading-relaxed" data-testid="reklama-sostoyanie">
          {stroki.map((s) => (
            <p key={s.tekst} className={s.vazhnoe ? "text-warn" : "text-fg-muted"} data-testid={s.vazhnoe ? "reklama-vazhnoe" : undefined}>
              {s.tekst}
            </p>
          ))}
        </div>
      )}

      <div className="flex flex-col gap-2">
        <h3 className="text-foreground text-[17px] font-semibold leading-tight">Список</h3>
        {/* Обёртка держит сегмент по содержимому: в колонке flex он
            растягивался на всю ширину с пустой рамкой справа. */}
        <div>
          <Segment<UrovenReklamy>
            aria-label="Список"
            ton="tihiy"
            aktiven={!disabled && reklama.vkl}
            vybrano={reklama.uroven}
            naVybor={(u) => naSmenu({ ...reklama, uroven: u })}
            znacheniya={UROVNI.map(({ z, podpis }) => ({ z, podpis }))}
          />
        </div>
        {uroven?.opisanie && <p className="text-fg-muted text-[13px] leading-relaxed">{uroven.opisanie}</p>}
      </div>

      <div className="flex flex-col gap-3">
        <header>
          <h3 className="text-foreground text-[17px] font-semibold leading-tight">Не блокировать</h3>
          <p className="text-fg-muted mt-1 text-[13px] leading-relaxed">
            Сайт или приложение перестало работать? Добавь адрес сюда. Правило действует на сайт и все его
            поддомены. Если браузер ещё помнит старый ответ, перезапусти его
          </p>
        </header>
        {/* Форма ради Enter в поле: вводит только черновик, поэтому случайное
            нажатие ничего не применяет. */}
        <form onSubmit={(e) => { e.preventDefault(); dobavit(); }} className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <Pole
              aria-label="Сайт-исключение"
              priv={pole}
              znachenie={vvod}
              aktiven={!disabled}
              naVvod={(v) => { setVvod(v); setUdalit(null); }}
              placeholder="example.org, пример.рф"
              className="flex-1"
            />
            <Knopka rang="vtoraya" tip="submit" aktiven={!disabled && razbor.gotovye.length > 0}>
              Добавить в черновик
            </Knopka>
          </div>
          {vvod.trim() !== "" && (
            <div className="flex flex-col gap-1.5 text-[13px]" data-testid="razbor-isklyucheniy">
              {razbor.gotovye.length > 0 && (
                <>
                  <p className="text-fg-muted">
                    Добавится {razbor.gotovye.length} {slovoPosleChisla(razbor.gotovye.length, "исключение", "исключения", "исключений")}:
                  </p>
                  <ul className="flex flex-col gap-0.5">
                    {razbor.gotovye.slice(0, POKAZAT_V_PREDPROSMOTRE).map((g) => (
                      <li key={g.domen} className="text-fg-secondary break-all">
                        {g.domen}
                        {g.ishodnyy.toLowerCase() !== g.domen && <span className="text-fg-muted"> · набрано {g.ishodnyy}</span>}
                        {reklama.razresheno.includes(g.domen) && <span className="text-fg-muted"> · уже в списке</span>}
                      </li>
                    ))}
                  </ul>
                  {razbor.gotovye.length > POKAZAT_V_PREDPROSMOTRE && (
                    <p className="text-fg-muted">и ещё {razbor.gotovye.length - POKAZAT_V_PREDPROSMOTRE}</p>
                  )}
                </>
              )}
              {razbor.otkazy.length > 0 && (
                <ul className="flex flex-col gap-0.5" role="alert">
                  {razbor.otkazy.slice(0, POKAZAT_V_PREDPROSMOTRE).map((o) => (
                    <li key={o.vvod} className="text-danger break-all">{o.vvod}: {o.prichina}</li>
                  ))}
                  {razbor.otkazy.length > POKAZAT_V_PREDPROSMOTRE && (
                    <li className="text-danger">и ещё {razbor.otkazy.length - POKAZAT_V_PREDPROSMOTRE} негодных</li>
                  )}
                </ul>
              )}
            </div>
          )}
        </form>

        {reklama.razresheno.length > 0 && (
          <ul aria-label="Исключения">
            {reklama.razresheno.map((d) => (
              <li key={d} className="border-border hover:bg-surface-hover flex items-center gap-3 border-b px-3 py-2.5 transition-colors">
                <span className="border-border bg-elevated text-fg-muted flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border">
                  <IkSayt className="h-4 w-4" />
                </span>
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="text-foreground truncate text-sm font-medium" title={d}>{d}</span>
                  <span className="text-fg-muted text-[13px]">Включая поддомены</span>
                </span>
                <Knopka
                  rang={udalit === d ? "opasnaya" : "tekst"}
                  aria-label={udalit === d ? `Подтвердить удаление ${d}` : `Удалить исключение ${d}`}
                  aktiven={!disabled}
                  onClick={() => {
                    if (udalit === d) {
                      naSmenu({ ...reklama, razresheno: reklama.razresheno.filter((x) => x !== d) });
                      setUdalit(null);
                      // Строка вместе с кнопкой сейчас исчезнет.
                      pole.current?.focus();
                    } else setUdalit(d);
                  }}
                >
                  {udalit === d ? "Подтвердить" : "Удалить"}
                </Knopka>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="flex flex-col">
        {/* Просьба владельца 29.09.2026: без этого раздела непонятно, зачем
            включать блокировку, если в браузере уже стоит расширение. */}
        <Svorachivaemyy
          zagolovok="Зачем включать"
          deti={
            <ul className="flex list-disc flex-col gap-1.5 pl-4">
              <li>Работает для всего компьютера: приложения, игры, мессенджеры и любой браузер, а не одна вкладка с расширением</li>
              <li>Режет трекеры и сбор данных в программах, куда расширение браузера не достаёт</li>
              <li>Запрос к рекламе не уходит вовсе: страницы открываются быстрее, трафика меньше</li>
              <li>Расширению нужен доступ ко всем открытым страницам, а здесь блокировка видит только имя сайта</li>
              <li>Расширение можно оставить: оно убирает пустые места и рекламу YouTube, вдвоём они закрывают больше</li>
            </ul>
          }
        />
        <Svorachivaemyy
          zagolovok="Что блокировка не умеет"
          deti={
            <ul className="flex list-disc flex-col gap-1.5 pl-4">
              <li>Реклама, которая идёт с тех же адресов, что и сам сервис: YouTube, Twitch, соцсети</li>
              <li>Пустые места на странице, где стояла реклама</li>
              <li>Браузер с вручную включённым безопасным DNS спрашивает адреса сам, мимо списка</li>
              <li>Правило во вкладках «Сайты» или «Сервисы» блокировку не снимает, для этого есть «Не блокировать»</li>
            </ul>
          }
        />
        <Svorachivaemyy
          zagolovok="Источник"
          deti={
            <p>
              Списки HaGeZi DNS Blocklists, лицензия GPL-3.0: базовый это Multi LIGHT, расширенный это Multi NORMAL.
              Служба скачивает свежий раз в сутки и перед заменой проверяет его. Если скачать не удалось, действует
              прежний список
            </p>
          }
        />
      </div>
    </div>
  );
}
