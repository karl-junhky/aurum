// «Настроить»: окно настроек в духе macOS. Слева разделы, справа строки с
// переключателями. Оформление, фон со своим видео или фото, автопроверка.
import { useEffect, useRef, useState } from "react";
import { PALITRY_PREVIEW } from "./Fon";
import { Tumbler, Vypadayushchiy } from "./elementy";
import { naNastroyki, naPravila, naSvoyVpn } from "./dom";
import { MAX_VIDEO_MB, sohranitMediaFayl, useUstanovki, zadat, type Akcent, type Fon, type Palitra, type Plotnost } from "./ustanovki";
import { IkPapka, IkPlaneta, IkPravila, IkShesterenka, IkZakryt, IkMolniya, IkTochki } from "./znachki";

const FONY: { z: Fon; podpis: string }[] = [
  { z: "perelivy", podpis: "Переливы" },
  { z: "planeta", podpis: "Планета" },
  { z: "galaktika", podpis: "Galaxy" },
  { z: "izgib", podpis: "Color Bends" },
  { z: "gradient", podpis: "Сияние" },
  { z: "net", podpis: "Нет" },
  { z: "svoy", podpis: "Своё видео / фото" },
];
const AKCENTY: { z: Akcent; podpis: string; c: string }[] = [
  { z: "zoloto", podpis: "Золото", c: "#e7c27d" },
  { z: "led", podpis: "Лёд", c: "#8fd3ff" },
  { z: "ametist", podpis: "Аметист", c: "#a78bfa" },
  { z: "izumrud", podpis: "Изумруд", c: "#5eead4" },
  { z: "roza", podpis: "Роза", c: "#fb7185" },
];
const PALITRY: { z: Palitra; podpis: string }[] = [
  { z: "okean", podpis: "Океан" }, { z: "zakat", podpis: "Закат" }, { z: "avrora", podpis: "Аврора" }, { z: "zoloto", podpis: "Золото" },
];
const PLOTNOSTI: { z: Plotnost; podpis: string }[] = [{ z: "steklo", podpis: "Стекло" }, { z: "tuman", podpis: "Туман" }, { z: "mat", podpis: "Мат" }];
const INTERVALY = [{ z: 0, podpis: "Выключена" }, { z: 2, podpis: "Каждые 2 мин" }, { z: 5, podpis: "Каждые 5 мин" }, { z: 10, podpis: "Каждые 10 мин" }, { z: 30, podpis: "Каждые 30 мин" }];

type Razdel = "vid" | "servery";

export function Panel({ zakryt }: { zakryt: () => void }) {
  const u = useUstanovki();
  const vhod = useRef<HTMLInputElement>(null);
  const [oshibka, setOshibka] = useState("");
  const [razdel, setRazdel] = useState<Razdel>("vid");
  useEffect(() => {
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") zakryt(); };
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [zakryt]);

  const zagruzit = async (f?: File) => {
    if (!f) return;
    try { await sohranitMediaFayl(f); setOshibka(""); }
    catch (e) { setOshibka(e instanceof Error ? e.message : "не получилось загрузить файл"); }
  };
  const vPolnoeOkno = (deystvie: () => void) => { zakryt(); zadat({ gud: false }); setTimeout(deystvie, 60); };

  return (
    <div className="aurum-settings-scrim" onPointerDown={(e) => { if (e.target === e.currentTarget) zakryt(); }}>
      <section className="aurum-settings-overlay" role="dialog" aria-modal="true" aria-label="Оформление" data-testid="aurum-panel">
        <aside className="aurum-settings-nav">
          <div className="aurum-settings-nav-top"><b>Настройки</b></div>
          <button type="button" className={`aurum-nav-item ${razdel === "vid" ? "is-active" : ""}`} onClick={() => setRazdel("vid")}><IkShesterenka /> Персонализация</button>
          <button type="button" className={`aurum-nav-item ${razdel === "servery" ? "is-active" : ""}`} onClick={() => setRazdel("servery")}><IkMolniya /> Серверы</button>
          <button type="button" className="aurum-nav-item" onClick={() => vPolnoeOkno(naPravila)}><IkPravila /> Правила и маршруты</button>
          <button type="button" className="aurum-nav-item" onClick={() => vPolnoeOkno(naSvoyVpn)}><IkPlaneta /> Свой VPN</button>
          <button type="button" className="aurum-nav-item" onClick={() => vPolnoeOkno(naNastroyki)}><IkTochki /> Дополнительно</button>
        </aside>

        <div className="aurum-settings-body">
          <header className="aurum-settings-header">
            <h3>{razdel === "vid" ? "Персонализация" : "Серверы"}</h3>
            <button type="button" className="aurum-ik-knopka" onClick={zakryt} aria-label="Закрыть настройки"><IkZakryt /></button>
          </header>

          {razdel === "vid" && (
            <div className="aurum-settings-rows">
              <h4>Фон</h4>
              <div className="aurum-plitki" role="radiogroup" aria-label="Фон">
                {FONY.map((f) => (
                  <button key={f.z} type="button" role="radio" aria-checked={u.fon === f.z} data-prev={f.z} className="aurum-plitka"
                    onClick={() => { zadat({ fon: f.z }); if (f.z === "svoy" && !u.mediaUrl) setTimeout(() => vhod.current?.click(), 80); }}>
                    <span className="aurum-plitka-kartinka" style={f.z === "svoy" && u.mediaUrl && u.tipMedia === "image" ? { backgroundImage: `url("${u.mediaUrl}")` } : undefined} />
                    <span>{f.podpis}</span>
                  </button>
                ))}
              </div>

              {u.fon === "perelivy" && (
                <div className="aurum-settings-row">
                  <span>Палитра</span>
                  <div className="aurum-palitry" role="radiogroup" aria-label="Палитра">
                    {PALITRY.map((p) => (
                      <button key={p.z} type="button" role="radio" aria-checked={u.palitra === p.z} aria-label={p.podpis} title={p.podpis}
                        style={{ background: `linear-gradient(135deg, ${PALITRY_PREVIEW[p.z][1]}, ${PALITRY_PREVIEW[p.z][2]} 60%, ${PALITRY_PREVIEW[p.z][3]})` }}
                        onClick={() => zadat({ palitra: p.z })} />
                    ))}
                  </div>
                </div>
              )}
              {u.fon === "svoy" && (
                <div className="aurum-custom-file-row">
                  <span className="aurum-file-path" title={u.imyaFayla}>{u.imyaFayla || "Файл не выбран"}</span>
                  <button type="button" className="aurum-replace-btn" onClick={() => vhod.current?.click()}><IkPapka /> {u.mediaUrl ? "Заменить" : "Выбрать файл"}</button>
                </div>
              )}
              <input ref={vhod} type="file" accept="video/mp4,video/webm,video/quicktime,image/*" hidden onChange={(e) => { void zagruzit(e.target.files?.[0]); e.target.value = ""; }} />
              {u.fon === "svoy" && <p className="aurum-podpis-nizu">Видео mp4 или webm до {MAX_VIDEO_MB} МБ, фото любого размера. Файл остаётся на этом компьютере.</p>}
              {oshibka && <p role="alert" className="aurum-oshibka">{oshibka}</p>}

              <div className="aurum-settings-divider" />
              <div className="aurum-settings-row">
                <span>Акцент</span>
                <div className="aurum-tochki-akcenta" role="radiogroup" aria-label="Акцент">
                  {AKCENTY.map((a) => (
                    <button key={a.z} type="button" role="radio" aria-checked={u.akcent === a.z} aria-label={a.podpis} title={a.podpis} style={{ ["--c" as string]: a.c }} onClick={() => zadat({ akcent: a.z })} />
                  ))}
                </div>
              </div>
              <div className="aurum-settings-row">
                <span>Материал</span>
                <Vypadayushchiy ariaLabel="Материал" znachenie={u.plotnost} varianty={PLOTNOSTI} naVybor={(plotnost) => zadat({ plotnost })} />
              </div>
              <div className="aurum-settings-row">
                <span>Яркость фона <i className="aurum-znach">{u.yarkost}%</i></span>
                <input type="range" className="aurum-range" min={20} max={100} value={u.yarkost} aria-label="Яркость фона" onChange={(e) => zadat({ yarkost: +e.target.value })} />
              </div>
              <div className="aurum-settings-row"><span>Зерно плёнки</span><Tumbler on={u.zerno} onChange={(zerno) => zadat({ zerno })} label="Зерно плёнки" /></div>
              <div className="aurum-settings-row"><span>Скорость под пилюлей</span><Tumbler on={u.pokazatSkorost} onChange={(pokazatSkorost) => zadat({ pokazatSkorost })} label="Скорость под пилюлей" /></div>
              <div className="aurum-settings-row"><span>Режим «пилюля»</span><Tumbler on={u.gud} onChange={(gud) => zadat({ gud })} label="Режим пилюля" /></div>
            </div>
          )}

          {razdel === "servery" && (
            <div className="aurum-settings-rows">
              <div className="aurum-settings-row">
                <span>Автопроверка пинга<small>Сама просит Affory измерить отклик серверов</small></span>
                <Vypadayushchiy ariaLabel="Автопроверка пинга" znachenie={u.avtoMin} varianty={INTERVALY} naVybor={(avtoMin) => zadat({ avtoMin })} />
              </div>
              <div className="aurum-settings-row">
                <span>Автовыбор лучшей страны<small>Переходит, если другая страна быстрее на треть и минимум на 20 мс</small></span>
                <Tumbler on={u.avtoVybor} onChange={(avtoVybor) => zadat({ avtoVybor })} label="Автовыбор лучшей страны" />
              </div>
              <p className="aurum-podpis-nizu">Лучшей считается страна с самым низким пингом, а разброс между её серверами и неотвечающие узлы немного снижают оценку. Рейтинг виден в панели «Серверы».</p>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
