// AURUM: точка входа. Единственная строка в файлах апстрима для самого вида это
// `import "./skin"` в main.tsx (и media-src в csp.ts для своего видео-фона);
// всё остальное лежит в этой папке, и при слиянии с новыми версиями Affory
// конфликтовать не с чем.
import ReactDOM from "react-dom/client";
import { useEffect } from "react";
import "./skin.css";
import { Fon } from "./Fon";
import { Gud } from "./Gud";
import { useAffory } from "./dom";
import { useAvto } from "./avto";
import { IkPilyulya } from "./znachki";
import { primenit, useUstanovki, vosstanovitMediaFayl, zadat } from "./ustanovki";

const NE_TASHCHIT = { ["--wails-draggable" as string]: "no-drag" } as React.CSSProperties;

function Obolochka() {
  const u = useUstanovki();
  const a = useAffory();
  useAvto(a);
  // Вариант оформления это атрибуты на <html>: CSS читает их, React не нужен.
  useEffect(() => { primenit(u); }, [u]);
  return (
    <>
      <Fon />
      {u.gud ? (
        <Gud a={a} />
      ) : (
        <button type="button" className="aurum-v-pilyulyu" style={NE_TASHCHIT} onClick={() => zadat({ gud: true })} aria-label="Режим «пилюля»" title="Режим «пилюля»" data-testid="aurum-v-pilyulyu">
          <IkPilyulya />
        </button>
      )}
    </>
  );
}

/** Название программы в окне: на экранах оно записано «Affory», наше имя
 *  подменяется на месте, без правок чужих файлов. Ядро, служба и журналы
 *  по-прежнему называются Affory. */
function pereimenovat() {
  document.querySelectorAll<HTMLElement>(".af-brand").forEach((el) => {
    el.childNodes.forEach((n) => {
      if (n.nodeType === Node.TEXT_NODE && n.textContent?.trim() === "Affory") n.textContent = "AURUM";
    });
  });
  if (document.title === "Affory") document.title = "AURUM";
}

function zapustit() {
  primenit();
  const host = document.createElement("div");
  host.id = "aurum-root";
  document.body.prepend(host);
  ReactDOM.createRoot(host).render(<Obolochka />);
  new MutationObserver(pereimenovat).observe(document.body, { subtree: true, childList: true });
  pereimenovat();
  void vosstanovitMediaFayl();
}

if (typeof document !== "undefined" && !document.getElementById("aurum-root")) {
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", zapustit);
  else zapustit();
}
