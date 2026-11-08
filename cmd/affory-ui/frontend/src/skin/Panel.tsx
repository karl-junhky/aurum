// «Настроить»: тема, фон, стекло. В референсе это три выпадающих строки; здесь
// плитки с живым предпросмотром, потому что фон проще выбрать глазами.
import { useRef, useState } from "react";
import { useUstanovki, zadat, szhatKartinku, type Akcent, type Fon, type Plotnost } from "./ustanovki";
import { IkPapka, IkZakryt } from "./znachki";

const FONY: { z: Fon; podpis: string }[] = [
  { z: "net", podpis: "Нет" },
  { z: "planeta", podpis: "Планета" },
  { z: "galaktika", podpis: "Galaxy" },
  { z: "izgib", podpis: "Color Bends" },
  { z: "gradient", podpis: "Сияние" },
  { z: "svoy", podpis: "Свой фон" },
];
const AKCENTY: { z: Akcent; podpis: string; c: string }[] = [
  { z: "zoloto", podpis: "Золото", c: "#e7c27d" },
  { z: "led", podpis: "Лёд", c: "#8fd3ff" },
  { z: "ametist", podpis: "Аметист", c: "#a78bfa" },
  { z: "izumrud", podpis: "Изумруд", c: "#5eead4" },
  { z: "roza", podpis: "Роза", c: "#fb7185" },
];
const PLOTNOSTI: { z: Plotnost; podpis: string }[] = [
  { z: "steklo", podpis: "Стекло" },
  { z: "tuman", podpis: "Туман" },
  { z: "mat", podpis: "Мат" },
];

export function Panel({ zakryt }: { zakryt: () => void }) {
  const u = useUstanovki();
  const vhod = useRef<HTMLInputElement>(null);
  const [oshibka, setOshibka] = useState("");
  const zagruzit = async (f?: File) => {
    if (!f) return;
    try { zadat({ kartinka: await szhatKartinku(f), fon: "svoy" }); setOshibka(""); }
    catch (e) { setOshibka(e instanceof Error ? e.message : "не получилось"); }
  };
  return (
    <section className="aurum-panel" role="dialog" aria-label="Оформление" data-testid="aurum-panel">
      <header>
        <b>Оформление</b>
        <button type="button" className="aurum-ik" onClick={zakryt} aria-label="Закрыть"><IkZakryt /></button>
      </header>

      <h4>Фон</h4>
      <div className="aurum-plitki" role="radiogroup" aria-label="Фон">
        {FONY.map((f) => (
          <button key={f.z} type="button" role="radio" aria-checked={u.fon === f.z} data-prev={f.z} className="aurum-plitka"
            onClick={() => (f.z === "svoy" && !u.kartinka ? vhod.current?.click() : zadat({ fon: f.z }))}>
            <span className="aurum-plitka-kartinka" style={f.z === "svoy" && u.kartinka ? { backgroundImage: `url("${u.kartinka}")` } : undefined} />
            <span>{f.podpis}</span>
          </button>
        ))}
      </div>
      <button type="button" className="aurum-faylik" onClick={() => vhod.current?.click()}>
        <IkPapka /> {u.kartinka ? "Заменить свою картинку" : "Выбрать свою картинку"}
      </button>
      <input ref={vhod} type="file" accept="image/*" hidden onChange={(e) => { void zagruzit(e.target.files?.[0]); e.target.value = ""; }} />
      {oshibka && <p role="alert" className="aurum-oshibka">{oshibka}</p>}

      <h4>Акцент</h4>
      <div className="aurum-tochki-akcenta" role="radiogroup" aria-label="Акцент">
        {AKCENTY.map((a) => (
          <button key={a.z} type="button" role="radio" aria-checked={u.akcent === a.z} aria-label={a.podpis} title={a.podpis} style={{ ["--c" as string]: a.c }} onClick={() => zadat({ akcent: a.z })} />
        ))}
      </div>

      <h4>Материал</h4>
      <div className="aurum-seg aurum-seg-mal" role="radiogroup" aria-label="Материал" data-n="3" data-aktiv={u.plotnost}>
        <span className="aurum-seg-thumb" aria-hidden />
        {PLOTNOSTI.map((p) => (
          <button key={p.z} type="button" role="radio" aria-checked={u.plotnost === p.z} onClick={() => zadat({ plotnost: p.z })}>{p.podpis}</button>
        ))}
      </div>

      <label className="aurum-polzunok">
        <span>Яркость фона <i>{u.yarkost}%</i></span>
        <input type="range" min={20} max={100} value={u.yarkost} onChange={(e) => zadat({ yarkost: +e.target.value })} />
      </label>
      <label className="aurum-pereklyuchatel">
        <span>Зерно плёнки</span>
        <input type="checkbox" className="tumbler" checked={u.zerno} onChange={(e) => zadat({ zerno: e.target.checked })} />
      </label>
      <label className="aurum-pereklyuchatel">
        <span>Режим «пилюля»</span>
        <input type="checkbox" className="tumbler" checked={u.gud} onChange={(e) => zadat({ gud: e.target.checked })} />
      </label>
    </section>
  );
}
