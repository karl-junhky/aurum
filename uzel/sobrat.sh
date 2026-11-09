#!/usr/bin/env bash
# Сборка vpn-uzel для linux/amd64.
#
# От обычного `go build` отличается одним: quic-go берётся с правкой скорости
# hy2, той же, что у ядра клиента (ustanovka/patches/quic-go-applimited.patch).
# Без неё BBR сервера после долгой отдачи разгоняет приём секунд пятнадцать с
# 3 Мбит/с.
#
# Правка накладывается на копию модуля во временном каталоге и подключается
# временным go.mod (-modfile). Рабочий go.mod её не знает: тесты и разработка
# идут на чистом quic-go, а бинарь для серверов всегда с правкой.
#
# Тулчейн go1.26.8 и теги те же, что у проверенного серверного ядра
# 1.14.2-affory-quic.1: программа должна отличаться от него кодом входов, а не
# компилятором.
#
# Выход: sborka/vpn-uzel и sborka/vpn-uzel.otpechatok.json.
set -euo pipefail

if [[ "${BASH_VERSINFO[0]}" -lt 4 ]]; then
  echo "нужен bash 4+" >&2
  exit 1
fi

KOREN="$(cd "$(dirname "$0")" && pwd)"
cd "$KOREN"
KUDA="${KUDA:-$KOREN/sborka}"
QUIC_VERSIYA="v0.61.0-sing-box-mod.7"
SINGBOX_VERSIYA="v1.14.2"
VERSIYA_YADRA="1.14.2-affory-quic.1"
TEGI="with_quic,with_utls,badlinkname,tfogo_checklinkname0"
PATCH="$KOREN/../ustanovka/patches/quic-go-applimited.patch"
export GOTOOLCHAIN=go1.26.8

skazat() { echo "== $*"; }

goversiya="$(go version)"
[[ "$goversiya" == *"go1.26.8 "* ]] || { echo "заказан go1.26.8, а go отвечает: $goversiya" >&2; exit 1; }

# Правка проверена на одной версии quic-go. Сменилась версия в sing-box -
# правку надо перепроверить, а не накладывать вслепую.
trebuetsya="$(go list -m -f '{{.Version}}' github.com/sagernet/quic-go)"
[[ "$trebuetsya" == "$QUIC_VERSIYA" ]] || { echo "модуль требует quic-go $trebuetsya, правка проверена на $QUIC_VERSIYA" >&2; exit 1; }
singbox="$(go list -m -f '{{.Version}}' github.com/sagernet/sing-box)"
[[ "$singbox" == "$SINGBOX_VERSIYA" ]] || { echo "sing-box $singbox, копии входов сверены с $SINGBOX_VERSIYA" >&2; exit 1; }

skazat "проверка и тесты"
go vet -tags with_quic,with_utls ./...
# В пробной сборке на CI (AURUM_BEZ_TESTOV=1) пропускаем тест таймингов отзыва:
# на медленном раннере он падает по времени, код тут ни при чём. Полный прогон остаётся.
PROPUSK=()
[[ -n "${AURUM_BEZ_TESTOV:-}" ]] && PROPUSK=(-skip TestOtzyvBezDochityvaniya)
go test -tags with_quic,with_utls -count=1 "${PROPUSK[@]}" ./...

# Копия quic-go лежит внутри sborka/ и подключается относительным путём: путь
# замены попадает в сведения о сборке бинаря, и абсолютный унёс бы туда имя
# пользователя и временный каталог.
RAB="$(mktemp -d)"
KOPIYA="$KUDA/.quic-go"
trap 'rm -rf "$RAB" "$KOPIYA"' EXIT
go mod download "github.com/sagernet/quic-go@$QUIC_VERSIYA"
iskhodnik="$(go env GOMODCACHE)/github.com/sagernet/quic-go@$QUIC_VERSIYA"
[[ -f "$iskhodnik/go.mod" ]] || { echo "quic-go не нашёлся в кэше модулей: $iskhodnik" >&2; exit 1; }
mkdir -p "$KUDA"
rm -rf "$KOPIYA"
cp -r "$iskhodnik" "$RAB/quic-go"
# Файлы кэша модулей только для чтения, копия наследует этот флаг.
chmod -R u+w "$RAB/quic-go"
skazat "правка quic-go $QUIC_VERSIYA"
# Правка накладывается ВНЕ репозитория, во временном каталоге, и только потом
# копия переезжает в sborka/. Внутри git-дерева `git apply` отсчитывает пути
# от его корня и МОЛЧА пропускает всё, что вне текущего каталога: проверка
# проходит, файл не меняется. Так 15.10 собрался бинарь без правки, и приём
# после отдачи на hy2 упал до 40 Мбит/с против 400.
# autocrlf выключен явно: иначе git apply переписал бы файл на CRLF, и копия
# разошлась бы с проверенной побайтно.
(cd "$RAB/quic-go" && git -c core.autocrlf=false apply --check "$PATCH" && git -c core.autocrlf=false apply "$PATCH")
grep -q 'if isAckEliciting && !h.packerEmptyTime.IsZero() {' "$RAB/quic-go/internal/ackhandler/sent_packet_handler.go" \
  || { echo "правка quic-go не легла в sent_packet_handler.go" >&2; exit 1; }
[[ -f "$RAB/quic-go/internal/ackhandler/affory_applimited_test.go" ]] || { echo "тест правки quic-go не лёг" >&2; exit 1; }
mv "$RAB/quic-go" "$KOPIYA"
# Тест из самой правки: подтверждение без данных не обрывает простой. Строка на
# месте ещё не значит, что поведение исправлено.
(cd "$KOPIYA" && go test -count=1 -run TestAfforyAckOnlyPacketKeepsIdlePeriod ./internal/ackhandler/)

otnositelno="$(realpath --relative-to="$KOREN" "$KOPIYA")"
cp go.mod "$RAB/sborka.mod"
cp go.sum "$RAB/sborka.sum"
go mod edit -modfile="$RAB/sborka.mod" -replace="github.com/sagernet/quic-go=./$otnositelno"

ld_apstrima="-X runtime.godebugDefault=multipathtcp=0,tlssha1=1 -checklinkname=0"
mkdir -p "$KUDA"
skazat "сборка linux/amd64, теги $TEGI"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -modfile="$RAB/sborka.mod" -trimpath -buildvcs=false -tags "$TEGI" \
  -ldflags "-X 'github.com/sagernet/sing-box/constant.Version=$VERSIYA_YADRA' $ld_apstrima -s -w -buildid=" \
  -o "$KUDA/vpn-uzel" ./cmd/vpn-uzel

# Правка обязана быть в бинаре: строка из её комментария в бинарь не попадёт,
# поэтому проверяется, что сборка шла с replace на копию.
go version -m "$KUDA/vpn-uzel" | grep -qE "=>[[:space:]]+\./.*quic-go" || { echo "в бинаре quic-go без правки" >&2; exit 1; }

sha() { sha256sum "$1" | cut -d' ' -f1; }
versiya="$(sed -n 's/^var Versiya = "\(.*\)"$/\1/p' cmd/vpn-uzel/main.go)"
printf '{\n  "programma": "vpn-uzel %s",\n  "sing_box": "%s",\n  "versiya_yadra": "%s",\n  "quic_go": "%s",\n  "patch_sha256": "%s",\n  "tegi": "%s",\n  "go": "%s",\n  "sha256": "%s",\n  "sobrano": "%s"\n}\n' \
  "$versiya" "$singbox" "$VERSIYA_YADRA" "$QUIC_VERSIYA" "$(sha "$PATCH")" "$TEGI" "$goversiya" \
  "$(sha "$KUDA/vpn-uzel")" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$KUDA/vpn-uzel.otpechatok.json"
skazat "готово: $KUDA/vpn-uzel"
cat "$KUDA/vpn-uzel.otpechatok.json"
