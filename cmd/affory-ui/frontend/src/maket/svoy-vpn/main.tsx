import React, { useState, type ReactNode } from "react";
import ReactDOM from "react-dom/client";
import "../../tokeny.css";
import "../../desktop.css";
import { IkNastroyki, IkPodklyuchenie, IkPravila, IkRazvernut, IkServer, IkSvernut, IkZakryt } from "../../ikonki";
import { ZnachokServisa } from "../../ekrany/ui";
import sphere from "../../assets/affory-sphere.png";
import github from "../../assets/github-white.svg";
import {
  KartochkaServera, ShagDostup, ShagGotovo, ShagProverka, ShagUstanovka, SvoyVpnMaster, SvoyVpnPusto, SvoyVpnServery,
  portSsh, type DeystvieServera, type SvoyServer,
} from "./SvoyVpn";
import * as D from "./dannye";

// Страница шаблона на стенде окна: `npx vite --config vite.stend.config.ts`,
// затем /maket-svoy-vpn.html?vid=<вид>. Кнопки водят по мастеру, вид в адресе
// открывает любое состояние сразу. Сама страница в выпуск не попадает (сборка
// берёт только index.html), но Tailwind обходит весь src, и классы шаблона
// уезжают в CSS выпуска. Поэтому каталог maket удаляется при переносе.

const VIDY = [
  "pusto", "dostup", "proverka", "proverka-otkaz", "ustanovka", "ustanovka-otkaz", "gotovo",
  "server", "server-vnimanie", "server-slomano", "server-bez-klyucha", "server-klyuch-hosta",
] as const;
type Vid = (typeof VIDY)[number];

function nachalnyyVid(): Vid {
  const v = new URLSearchParams(window.location.search).get("vid");
  return VIDY.find((x) => x === v) ?? "pusto";
}

// Полоса окна с четвёртым разделом. Копия разметки Karkas.tsx, а не сам
// Karkas: тот рисует вкладки из VKLADKI. При переносе это пункт Ж1 плана,
// правка vkladki.ts и ZNACHKI в Karkas.tsx, а эта копия уходит.
const RAZDELY = [
  { imya: "Подключение", Znachok: IkPodklyuchenie },
  { imya: "Правила", Znachok: IkPravila },
  { imya: "Свой VPN", Znachok: IkServer },
  { imya: "Настройки", Znachok: IkNastroyki },
];

function PolosaMaketa({ metka }: { metka?: "warn" | "danger" }) {
  return (
    <header className="af-titlebar border-border bg-rail flex h-polosa shrink-0 items-stretch justify-between border-b pl-4 select-none">
      <div className="af-title-left flex min-w-0 items-center gap-7">
        <span className="af-brand flex shrink-0 items-center gap-2.5 text-[15px] font-semibold tracking-tight">
          <img src={sphere} alt="" className="h-[22px] w-[22px]" />
          Affory
        </span>
        <nav role="tablist" aria-label="Разделы" className="flex h-polosa items-stretch">
          {RAZDELY.map(({ imya, Znachok }) => {
            const aktivna = imya === "Свой VPN";
            return (
              <button
                key={imya}
                type="button"
                role="tab"
                aria-selected={aktivna}
                tabIndex={aktivna ? 0 : -1}
                className={aktivna
                  ? "text-accent-ink relative flex items-center gap-2 px-3.5 text-sm font-medium"
                  : "text-fg-muted hover:text-fg-secondary relative flex items-center gap-2 px-3.5 text-sm font-medium"}
              >
                {aktivna && <span aria-hidden className="bg-fill-subtle absolute inset-x-1 inset-y-[7px] -z-10 rounded-md" />}
                <Znachok className="h-[18px] w-[18px]" />
                <span className={aktivna ? "text-foreground" : undefined}>{imya}</span>
                {/* Метка раздела: сломанный свой сервер виден с любой вкладки
                    (план, §4: «видны в общем статусе»). Точка, а не заливка. */}
                {imya === "Свой VPN" && metka && (
                  <>
                    <span aria-hidden className={`absolute top-[15px] right-2 h-1.5 w-1.5 rounded-full ${metka === "warn" ? "bg-warn" : "bg-danger"}`} />
                    <span className="sr-only">{metka === "warn" ? "нужно внимание" : "не работает"}</span>
                  </>
                )}
                {aktivna && <span aria-hidden className="bg-accent-ink absolute inset-x-1 bottom-0 h-[2px] rounded-t" />}
              </button>
            );
          })}
        </nav>
      </div>
      <div className="af-window-controls flex shrink-0 items-stretch">
        <span className="mr-1 flex items-center px-3"><ZnachokServisa src={github} className="h-4 w-[70px]" /></span>
        <span className="text-fg-muted flex w-11 items-center justify-center"><IkSvernut className="h-4 w-4" /></span>
        <span className="text-fg-muted flex w-11 items-center justify-center"><IkRazvernut className="h-4 w-4" /></span>
        <span className="text-fg-muted flex w-11 items-center justify-center"><IkZakryt className="h-4 w-4" /></span>
      </div>
    </header>
  );
}

const OTKAZ_PROVERKI = {
  zagolovok: "Сервер на OpenVZ, VPN на нём не встанет",
  tekst: "Нужна виртуализация KVM. Посмотри условия возврата у хостера или попроси перевести сервер на KVM",
};

const OTKAZ_USTANOVKI = {
  zagolovok: "Из твоей сети не прошёл ни один порт",
  tekst: "Сервер жив: через другой сервер он отвечает. Твоя сеть не пускает к его адресу. Попроси у хостера другой адрес и попробуй снова",
};

function Maket() {
  const [vid, zadatVid] = useState<Vid>(nachalnyyVid);
  const [dostup, zadatDostup] = useState(D.DOSTUP);
  // Согласие «сервер пустой» дано конкретной машине: сменил адрес, порт SSH
  // или пользователя на шаге доступа, и галочку надо ставить заново.
  const [pustoyDlya, zadatPustoyDlya] = useState<string | null>(null);
  const mashina = `${dostup.polzovatel.trim()}@${dostup.adres.trim()}:${portSsh(dostup)}`;
  const [vhod, zadatVhod] = useState(true);
  const idti = (v: Vid) => {
    zadatVid(v);
    window.history.replaceState(null, "", `?vid=${v}`);
  };
  const nichego = () => undefined;
  const adres = dostup.adres || undefined;
  const pustoy = pustoyDlya === mashina;

  // «Подобрать порты заново» и прочее из блока З идут своим ходом поверх
  // живого сервера (К4), а не мастером установки, поэтому на стенде молчат.
  const deystvie = (d: DeystvieServera) => {
    if (d === "proverit" || d === "pereustanovil") idti("server");
    else if (d === "vzyat") idti("dostup");
    else if (d === "zabyt") idti("pusto");
    else if (d === "vhod") zadatVhod(!vhod);
  };
  const servery = (s: SvoyServer) => (
    <SvoyVpnServery
      servery={[{ ...s, vhodPoParolyu: vhod }]}
      naPostavit={() => idti("dostup")}
      kartochka={(x) => <KartochkaServera server={x} naDeystvie={deystvie} />}
    />
  );

  let ekran: ReactNode;
  let pokazan: SvoyServer | undefined;
  switch (vid) {
    case "pusto":
      ekran = <SvoyVpnPusto naPostavit={() => idti("dostup")} />;
      break;
    case "dostup":
      ekran = (
        <SvoyVpnMaster shag="dostup" adres={adres}>
          <ShagDostup
            dostup={dostup}
            naSmenu={zadatDostup}
            buferSParolem
            naOchistitBufer={nichego}
            naVybratKlyuch={() => zadatDostup({ ...dostup, klyuchFayl: "id_ed25519" })}
            naOtmenu={() => idti("pusto")}
            naDalshe={() => idti("proverka")}
          />
        </SvoyVpnMaster>
      );
      break;
    case "proverka":
    case "proverka-otkaz": {
      const otkaz = vid === "proverka-otkaz" ? OTKAZ_PROVERKI : undefined;
      ekran = (
        <SvoyVpnMaster shag="proverka" adres={adres}>
          <ShagProverka
            punkty={otkaz ? D.OSMOTR_OTKAZ : D.OSMOTR}
            otpechatok={D.OTPECHATOK}
            otkaz={otkaz}
            pustoy={pustoy}
            naPustoy={(v) => zadatPustoyDlya(v ? mashina : null)}
            naNazad={() => idti("dostup")}
            naDalshe={() => idti("ustanovka")}
          />
        </SvoyVpnMaster>
      );
      break;
    }
    case "ustanovka":
    case "ustanovka-otkaz": {
      const otkaz = vid === "ustanovka-otkaz" ? OTKAZ_USTANOVKI : undefined;
      ekran = (
        <SvoyVpnMaster shag="ustanovka" adres={adres}>
          <ShagUstanovka
            etapy={otkaz ? D.ETAPY_OTKAZ : D.ETAPY}
            podbor={otkaz ? D.PODBOR_OTKAZ : D.PODBOR}
            otkaz={otkaz}
            naOtmenu={() => idti("pusto")}
            naOtchyot={nichego}
            naPovtor={() => idti("ustanovka")}
          />
          {!otkaz && (
            <button type="button" className="text-fg-faint self-start text-xs" onClick={() => idti("gotovo")}>
              стенд: завершить установку
            </button>
          )}
        </SvoyVpnMaster>
      );
      break;
    }
    case "gotovo":
      ekran = (
        <SvoyVpnMaster shag="gotovo" adres={adres}>
          <ShagGotovo klyuchi={D.SERVER.klyuchi} vhodPoParolyu={vhod} naPodklyuchit={() => idti("server")} naVygruzit={nichego} naVhod={() => zadatVhod(!vhod)} />
        </SvoyVpnMaster>
      );
      break;
    case "server":
      pokazan = D.SERVER;
      break;
    case "server-vnimanie":
      pokazan = D.SERVER_VNIMANIE;
      break;
    case "server-slomano":
      pokazan = D.SERVER_SLOMANO;
      break;
    case "server-bez-klyucha":
      pokazan = D.SERVER_BEZ_KLYUCHA;
      break;
    case "server-klyuch-hosta":
      pokazan = D.SERVER_KLYUCH_HOSTA;
      break;
    default: {
      // Новый вид в VIDY без своей ветки не соберётся здесь, а не покажет
      // молча чужой экран.
      const nevozmozhno: never = vid;
      throw new Error(`вид без экрана: ${String(nevozmozhno)}`);
    }
  }

  if (pokazan) ekran = servery(pokazan);
  // Метка раздела следует за состоянием сервера, а не задаётся видом стенда.
  const metka = pokazan?.sostoyanie === "vnimanie" ? "warn" : pokazan?.sostoyanie === "slomano" ? "danger" : undefined;

  return (
    <main className="affory-desktop bg-background text-foreground h-screen">
      <div className="flex h-screen min-w-0 flex-col">
        <PolosaMaketa metka={metka} />
        <div className="af-viewport flex min-h-0 min-w-0 flex-1 flex-col">{ekran}</div>
      </div>
    </main>
  );
}

const koren = document.getElementById("root");
if (!koren) throw new Error("в maket-svoy-vpn.html нет #root");

ReactDOM.createRoot(koren).render(
  <React.StrictMode>
    <Maket />
  </React.StrictMode>,
);
