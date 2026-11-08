// AURUM: точка входа. Единственная строка в файлах апстрима это
// `import "./skin"` в main.tsx; всё остальное лежит в этой папке и при слиянии
// с новыми версиями Affory конфликтовать не с чем.
import ReactDOM from "react-dom/client";
import { useEffect } from "react";
import "./skin.css";
import { Fon } from "./Fon";
import { Gud } from "./Gud";
import { IkPilyulya } from "./znachki";
import { primenit, useUstanovki, zadat, ustanovki, podpisatsya } from "./ustanovki";

const NE_TASHCHIT = { ["--wails-draggable" as string]: "no-drag" } as React.CSSProperties;

function Obolochka() {
  const u = useUstanovki();
  // Вариант оформления это атрибуты на <html>: CSS читает их, React не нужен.
  useEffect(() => { primenit(u); }, [u]);
  return (
    <>
      <Fon />
      {u.gud ? (
        <Gud />
      ) : (
        <button type="button" className="aurum-v-pilyulyu" style={NE_TASHCHIT} onClick={() => zadat({ gud: true })} aria-label="Режим «пилюля»" title="Режим «пилюля»" data-testid="aurum-v-pilyulyu">
          <IkPilyulya />
        </button>
      )}
    </>
  );
}

/** Название программы в окне: на экранах оно жёстко записано «Affory», а наше
 *  имя подменяется на месте, без правок чужих файлов. Ядро и служба
 *  по-прежнему называются Affory, и в журналах тоже. */
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
}

if (typeof document !== "undefined" && !document.getElementById("aurum-root")) {
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", zapustit);
  else zapustit();
}
// Для тестов и стенда.
export { ustanovki, zadat, podpisatsya };
