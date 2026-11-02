# Сборка выпуска: бинари, установщик NSIS, архив обновления с .sha256.
#
# Выпуск состоит из четырёх вещей:
#   * Affory-<версия>-setup.exe: установщик (ustanovka\affory.nsi);
#   * affory-<версия>.zip плюс affory-<версия>.zip.sha256: архив для команды
#     installUpdate: четыре файла верхнего уровня, ядро в том числе;
#   * sing-box-<версия ядра>.zip не собирается: ядро это своя сборка апстримного
#     sing-box (ustanovka\sobrat-yadro.ps1), оно кладётся в установщик как есть;
#   * vpn-uzel-<версия>-linux-amd64 с .sha256: программа сервера «Своего VPN»,
#     её служба качает отсюда при первой установке сервера.
#
# Ничего секретного в выпуск не попадает: ни адресов, ни ключей, ни подписки.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Versiya,
    [string]$Yadro = (Join-Path $PSScriptRoot 'yadro\sing-box.exe'),
    [string]$Makensis = 'C:\Program Files (x86)\NSIS\makensis.exe',
    # Отпечаток сертификата подписи. Без него выпуск собирается НЕПОДПИСАННЫМ
    # и громко об этом говорит: молчаливая сборка без подписи это ровно тот
    # случай, когда о ней забывают.
    [string]$Otpechatok = '',
    [switch]$BezFronta,
    # bash из Git для uzel/sobrat.sh. Голое `bash` на Windows бывает bash из
    # WSL, а тот собрал бы программу другим окружением.
    [string]$Bash = 'C:\Program Files\Git\bin\bash.exe',
    # Пробная сборка для стенда из дерева без ворот. Выкладывать её нельзя, и
    # в каталог выпуска кладётся файл, который об этом говорит.
    [switch]$Proba
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
if ($Versiya -notmatch '^\d+\.\d+\.\d+$') { throw "версия $Versiya не вида X.Y.Z" }
# Номер сверяется с деревом, а не принимается на слово.
#
# Собрать релиз с версией, которой в дереве нет, значит выложить сборку, чью
# родословную потом никто не восстановит: в exe одно число, в исходниках
# другое. Расхождение это ошибка ввода, и ловить её надо здесь, а не после
# выкладки.
$faylVersii = Join-Path (Split-Path $PSScriptRoot -Parent) 'VERSIYA'
if (Test-Path $faylVersii) {
    $vDereve = (Get-Content $faylVersii -Raw).Trim()
    if ($vDereve -ne $Versiya) {
        throw "версия $Versiya не совпадает с деревом ($vDereve): поправьте VERSIYA или аргумент"
    }
} else {
    throw "нет файла VERSIYA в корне дерева: собирать релиз вслепую нельзя"
}
# Б2 аудита 1.6.1: выпуск собирается только из дерева, прошедшего ворота.
. (Join-Path $PSScriptRoot 'otmetka-vorot.ps1')
$metkaProby = Join-Path $PSScriptRoot 'vypusk\NE-DLYA-VYKLADKI.txt'
if ($Proba) {
    Write-Host 'ПРОБНАЯ СБОРКА: ворота не проверены, выкладывать нельзя' -ForegroundColor Yellow
} else {
    Proverit-OtmetkuVorot (Split-Path $PSScriptRoot -Parent)
}
if (-not (Test-Path $Yadro)) { throw "нет ядра $Yadro" }
if (-not (Test-Path $Makensis)) { throw "нет makensis: $Makensis" }

$koren  = Split-Path $PSScriptRoot -Parent
# Б5 аудита 1.6.1: компилятор выпуска задан строкой toolchain в go.mod, а не
# тем, что оказалось на машине сборки. Версия уходит в паспорт выпуска.
$tulchein = (Select-String -Path (Join-Path $koren 'go.mod') -Pattern '^toolchain\s+(\S+)').Matches.Groups[1].Value
if (-not $tulchein) { throw 'в go.mod нет строки toolchain: компилятор выпуска не закреплён' }
$env:GOTOOLCHAIN = $tulchein
$versiyaGo = (& go env GOVERSION).Trim()
if ($LASTEXITCODE -ne 0 -or $versiyaGo -ne $tulchein) { throw "компилятор $versiyaGo вместо закреплённого $tulchein" }
Write-Host "компилятор: $versiyaGo" -ForegroundColor Cyan
$sborka = Join-Path $PSScriptRoot 'sborka'
$vypusk = Join-Path $PSScriptRoot 'vypusk'
$taskBuildPath = [IO.Path]::GetFullPath($sborka)
$taskExpectedPath = Join-Path ([IO.Path]::GetFullPath($PSScriptRoot)) 'sborka'
if ($taskBuildPath -ne $taskExpectedPath) { throw 'build cleanup escaped the installer workspace' }
if (Test-Path -LiteralPath $taskBuildPath) { Remove-Item -LiteralPath $taskBuildPath -Recurse -Force }
$null = New-Item -ItemType Directory -Path $sborka, $vypusk -Force
if ($Proba) {
    [IO.File]::WriteAllText($metkaProby, "Пробная сборка $Versiya без ворот, $(Get-Date -Format o). Не выкладывать.`n", [Text.UTF8Encoding]::new($false))
} elseif (Test-Path $metkaProby) {
    Remove-Item $metkaProby -Force
}

# Программа сервера «Своего VPN» (У5): клиент качает её из этого выпуска и
# сверяет с отпечатком, вшитым в службу. Она собирается здесь же из дерева
# выпуска: только так файл в выпуске соответствует исходникам uzel/ в теге и
# в архиве исходников. Сборка воспроизводима (02.11.2026 пересборка дала тот
# же sha256), поэтому несовпадение с вшитым значит, что исходники uzel/
# менялись без новой сборки и нового отпечатка. Всё это до сборки клиента,
# чтобы не ждать зря.
$otpUzla = Get-Content (Join-Path $koren 'cmd\affory-svc\vpn-uzel.otpechatok.json') -Raw | ConvertFrom-Json
if ($otpUzla.programma -notmatch '^vpn-uzel (\d+\.\d+\.\d+)$') { throw "в отпечатке программы сервера нет версии: $($otpUzla.programma)" }
$versiyaUzla = $Matches[1]
if (-not (Test-Path $Bash)) { throw "нет bash из Git для сборки программы сервера: $Bash" }
& $Bash (($koren -replace '\\', '/') + '/uzel/sobrat.sh')
if ($LASTEXITCODE -ne 0) { throw 'программа сервера не собралась (uzel/sobrat.sh)' }
$uzel = Join-Path $koren 'uzel\sborka\vpn-uzel'
$hu = (Get-FileHash $uzel -Algorithm SHA256).Hash.ToLower()
if ($hu -ne $otpUzla.sha256) {
    throw "программа сервера из дерева $hu, а в службе вшит $($otpUzla.sha256): переложить uzel\sborka\vpn-uzel.otpechatok.json в cmd\affory-svc"
}
# Потолок загрузки программы в службе 96 МБ (predelProgrammyUzla). Упрётся в
# него - установка своего сервера встанет у всех, а причина будет в журнале.
$razmerUzla = (Get-Item $uzel).Length
if ($razmerUzla -gt 90MB) { throw "программа сервера $([math]::Round($razmerUzla/1MB,1)) МБ, потолок загрузки в службе 96 МБ: поднимать надо и там" }
Get-ChildItem $vypusk -Filter 'vpn-uzel-*' | Remove-Item -Force
$imyaUzla = "vpn-uzel-$versiyaUzla-linux-amd64"
Copy-Item $uzel (Join-Path $vypusk $imyaUzla)
[IO.File]::WriteAllText((Join-Path $vypusk "$imyaUzla.sha256"), "$hu  $imyaUzla`n")

Push-Location $koren
try {
    if (-not $BezFronta) {
        Push-Location (Join-Path $koren 'cmd\affory-ui\frontend')
        try {
            # ErrorActionPreference сбрасывается ВОКРУГ npm намеренно.
            #
            # В Windows PowerShell 5.1 редирект `2>&1` на НАТИВНОЙ команде
            # оборачивает каждую строку stderr в ErrorRecord, и под
            # ErrorActionPreference=Stop безобидное «npm notice» про новую
            # версию убивает весь прогон. 04.09.2026 так оборвался полный
            # заход приёмки на первой же строке, ещё до единого судьи.
            # Успех сборки судится по коду выхода, а не по молчанию stderr.
            $prezhnee = $ErrorActionPreference
            $ErrorActionPreference = 'Continue'
            # npm ci ставит ровно то, что записано в package-lock.json (Б5 аудита
            # 1.6.1): голый build брал то, что случайно лежало в node_modules.
            try { & npm ci --no-audit --no-fund 2>&1 | Select-Object -Last 2 }
            finally { $ErrorActionPreference = $prezhnee }
            if ($LASTEXITCODE -ne 0) { throw 'зависимости фронта не поставились' }
            $ErrorActionPreference = 'Continue'
            try { & npm run build 2>&1 | Select-Object -Last 2 }
            finally { $ErrorActionPreference = $prezhnee }
            if ($LASTEXITCODE -ne 0) { throw 'фронт не собрался' }
        } finally { Pop-Location }
    }
    # -s -w: без таблицы символов и DWARF, выпуск легче на треть. -trimpath:
    # пути машины сборки в бинарь не попадают.
    $ld = "-s -w -X main.versiyaProgrammy=$Versiya"
    # Манифест и номер у службы и командной строки (1.10.0): Windows 10 в
    # совместимости, длинные пути, запуск без повышения. Длинные пути манифест
    # включает только вместе с LongPathsEnabled в реестре, поэтому глубокую
    # папку установки отсекает ещё и установщик. Файл ресурсов убирается
    # сразу после сборки, по той же причине, что и у окна ниже.
    foreach ($imya in 'affory-svc', 'affory-cli') {
        $paket = Join-Path $koren "cmd\$imya"
        Push-Location $paket
        try {
            & go run github.com/tc-hib/go-winres@v0.3.3 make --in winres\winres.json --out rsrc --arch amd64 --file-version $Versiya --product-version $Versiya
            if ($LASTEXITCODE -ne 0) { throw "ресурсы $imya не собрались" }
        } finally { Pop-Location }
        try {
            & go build -trimpath -ldflags $ld -o (Join-Path $sborka "$imya.exe") "./cmd/$imya"
            if ($LASTEXITCODE -ne 0) { throw "$imya не собрался" }
        } finally {
            Remove-Item (Join-Path $paket 'rsrc_windows_amd64.syso') -ErrorAction SilentlyContinue
        }
    }
    # Иконка, версия и манифест в ресурсах affory-ui.exe: go build сам
    # подхватывает rsrc_windows_amd64.syso из каталога пакета. Без него у exe
    # нет иконки ни в проводнике, ни в панели задач (выпуск 0.6.0 ушёл таким).
    # Масштаб в манифесте тот же, что Wails ставит сам при запуске
    # (permonitorv2): увидев его в манифесте, Wails свой вызов пропускает.
    # go-winres закреплённой версией, а не тем, что лежит в go\bin (Б5 аудита
    # 1.6.1). От его работы зависит номер в ресурсах, по которому служба
    # отвергает понижение версии.
    Push-Location (Join-Path $koren 'cmd\affory-ui')
    try {
        & go run github.com/tc-hib/go-winres@v0.3.3 make --in winres\winres.json --out rsrc --arch amd64 --file-version $Versiya --product-version $Versiya
        if ($LASTEXITCODE -ne 0) { throw 'ресурсы affory-ui не собрались' }
    } finally { Pop-Location }
    # -tags production: без метки Wails считает сборку отладочной и включает
    # ПОВЕРХ наших опций родное меню WebView2 (`debugMode || !DefaultContextMenuDisabled`)
    # и панель разработчика. Выпуски 1.0.0-1.1.3 ушли людям такими.
    #
    # Файл ресурсов убирается сразу после сборки. Оставленный, он вшивал номер
    # прошлого выпуска в каждую сборку окна через go build, и служба отвергала
    # такие архивы как понижение (28.09.2026, стенд после выпуска 1.6.2).
    try {
        & go build -trimpath -tags production -ldflags "$ld -H windowsgui" -o (Join-Path $sborka 'affory-ui.exe') ./cmd/affory-ui
        if ($LASTEXITCODE -ne 0) { throw 'affory-ui не собрался' }
    } finally {
        Remove-Item (Join-Path $koren 'cmd\affory-ui\rsrc_windows_amd64.syso') -ErrorAction SilentlyContinue
    }
    # Загрузчик WebView2 для установщика. Он уже лежит в модуле Wails той версии,
    # что записана в go.mod, и качать его отдельно незачем. Подпись Microsoft
    # сверяется: установщик запускает его с правами администратора.
    $wails = & go list -m -f '{{.Dir}}' github.com/wailsapp/wails/v3
    if ($LASTEXITCODE -ne 0 -or -not $wails) { throw 'каталог модуля Wails не найден' }
    $zagruzchik = Join-Path $wails 'internal\commands\webview2\MicrosoftEdgeWebview2Setup.exe'
    if (-not (Test-Path $zagruzchik)) { throw "в модуле Wails нет загрузчика WebView2: $zagruzchik" }
    $podpis = Get-AuthenticodeSignature $zagruzchik
    if ($podpis.Status -ne 'Valid' -or $podpis.SignerCertificate.Subject -notmatch '^CN=Microsoft Corporation,') {
        throw "у загрузчика WebView2 нет действительной подписи Microsoft: $($podpis.Status) $($podpis.SignerCertificate.Subject)"
    }
    Copy-Item $zagruzchik (Join-Path $sborka 'MicrosoftEdgeWebview2Setup.exe')
} finally { Pop-Location }
Copy-Item $Yadro (Join-Path $sborka 'sing-box.exe')

# Ядро выпуска обязано быть ОПОЗНАВАЕМЫМ.
#
# Выпуск 1.14.0-lx.1-affory собран с ветки форка, которой больше не существует,
# и вернуться к нему теперь можно только сохранённым бинарём: ни коммита, ни
# способа его узнать не записал никто. Отпечаток кладёт sobrat-yadro.ps1, и
# отсутствие отпечатка ОСТАНАВЛИВАЕТ выпуск: ядро, про которое нельзя сказать,
# из чего оно, уезжает людям ровно один раз, а разбираться приходится годами.
#
# Хеш сверяется с тем ядром, которое едет СЕЙЧАС: отпечаток от прошлой сборки
# рядом со свежим бинарём это враньё, причём убедительное.
$putOtpYadra = "$Yadro.otpechatok.json"
if (-not (Test-Path $putOtpYadra)) {
    throw "рядом с ядром нет отпечатка ($putOtpYadra). Пересобрать: ustanovka\sobrat-yadro.ps1"
}
$otpYadra = Get-Content $putOtpYadra -Raw -Encoding UTF8 | ConvertFrom-Json
$heshYadra = (Get-FileHash $Yadro -Algorithm SHA256).Hash.ToLower()
if ($otpYadra.sha256 -ne $heshYadra) {
    throw ("отпечаток описывает другое ядро (в файле $($otpYadra.sha256), на диске $heshYadra)." +
           ' Пересобрать: ustanovka\sobrat-yadro.ps1')
}
if (-not $otpYadra.kommit) { throw 'в отпечатке ядра нет коммита, воспроизвести сборку нечем' }
# Поле переименовано вместе с уходом с форка: раньше сборка бралась с ветки, теперь
# с тега. Пустое поле в ИСТОРИЧЕСКОЙ записи хуже отсутствующего: оно выглядит как
# ответ. Записи выпусков до 0.8.1 несут vetka, с 0.8.1 несут teg.
if (-not $otpYadra.teg) { throw 'в отпечатке ядра нет тега, запись в историю ушла бы с пустым полем' }

# А это уже В ИСТОРИЮ. Каталог ядра вне git, поэтому единственное место, где
# запись переживёт машину, это отслеживаемый файл рядом со скриптами выпуска.
# Хеш здесь от НЕПОДПИСАННОГО ядра: подпись ставится ниже и файл меняет, а
# воспроизводится сборкой именно неподписанный бинарь.
$zapis = [ordered]@{
    vypusk      = $Versiya
    repozitoriy = $otpYadra.repozitoriy
    teg         = $otpYadra.teg
    kommit      = $otpYadra.kommit
    process_family = $otpYadra.process_family
    quic_go     = $otpYadra.quic_go
    versiya     = $otpYadra.versiya
    tegi        = $otpYadra.tegi
    go          = $otpYadra.go
    sha256      = $otpYadra.sha256
    bayt        = $otpYadra.bayt
    sobrano     = $otpYadra.sobrano
    # Компилятор клиента, не ядра: ядро собирается своим скриптом, и его
    # версия выше в поле go.
    go_klienta  = $versiyaGo
}
$zapis | ConvertTo-Json -Depth 5 | Set-Content -Path (Join-Path $PSScriptRoot 'YADRO-VYPUSKA.json') -Encoding UTF8
Write-Host "ядро выпуска: $($otpYadra.versiya), коммит $($otpYadra.kommit)" -ForegroundColor Cyan

# Подпись ДО упаковки, иначе в установщик уедут подписанные файлы, а в архив
# обновления неподписанные, и после первого же самообновления продукт станет
# подписанным наполовину.
$podpisat = Join-Path $PSScriptRoot 'podpisat.ps1'
$kBinaryam = 'affory-svc.exe', 'affory-cli.exe', 'affory-ui.exe', 'sing-box.exe' |
    ForEach-Object { Join-Path $sborka $_ }
if ($Otpechatok) {
    & $podpisat -Fayly $kBinaryam -Otpechatok $Otpechatok
} else {
    Write-Host 'ВЫПУСК БЕЗ ПОДПИСИ: отпечаток не задан (-Otpechatok)' -ForegroundColor Yellow
}

# Архив обновления: только наши exe, формат тот же, что принимает installUpdate.
$zip = Join-Path $vypusk "affory-$Versiya.zip"
Remove-Item $zip -Force -ErrorAction SilentlyContinue
# Ядро едет В архиве с 0.7.0. До этого оно обновлялось только установщиком,
# то есть машина, обновлявшаяся архивом, годами сидела бы на старом ядре и
# никто бы этого не заметил. Архив от этого тяжелеет, и это честная плата.
Compress-Archive -Path (Join-Path $sborka 'affory-svc.exe'), (Join-Path $sborka 'affory-cli.exe'), (Join-Path $sborka 'affory-ui.exe'), (Join-Path $sborka 'sing-box.exe') -DestinationPath $zip
# Потолок загрузки по сети в службе 64 МБ (predelArhivaSeti). Упереться в него
# молча нельзя: обновление тогда перестанет ставиться, а причина будет в чужом
# файле.
$razmerZip = (Get-Item $zip).Length
if ($razmerZip -gt 60MB) { throw "архив обновления $([math]::Round($razmerZip/1MB,1)) МБ, потолок загрузки в службе 64 МБ: поднимать надо и там" }
$h = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
[IO.File]::WriteAllText("$zip.sha256", "$h  affory-$Versiya.zip`n")

# Установщик.
Push-Location $PSScriptRoot
try {
    $nsisKlyuchi = @("/V2", "/DVERSIYA=$Versiya", "/DSBORKA=sborka")
    if ($Otpechatok) {
        $taskSigningShell = (Get-Process -Id $PID).Path
        $nsisKlyuchi += @("/DOTPECHATOK=$Otpechatok", "/DPODPISAT=$podpisat", "/DSIGN_POWERSHELL=$taskSigningShell")
    }
    & $Makensis $nsisKlyuchi affory.nsi 2>&1 | Select-Object -Last 3
    if ($LASTEXITCODE -ne 0) { throw 'makensis завершился с ошибкой' }
} finally { Pop-Location }
$setup = Join-Path $vypusk "Affory-$Versiya-setup.exe"
if (-not (Test-Path $setup)) { throw "установщик не появился: $setup" }
# Подпись меняет байты, значит сумма считается ПОСЛЕ неё. Иначе .sha256 будет
# описывать файл, которого никто никогда не увидит.
if ($Otpechatok) { & $podpisat -Fayly $setup -Otpechatok $Otpechatok }
$hs = (Get-FileHash $setup -Algorithm SHA256).Hash.ToLower()
[IO.File]::WriteAllText("$setup.sha256", "$hs  Affory-$Versiya-setup.exe`n")

# Архив исходников.
#
# Собирается ЗДЕСЬ, а не руками рядом. При выпуске 1.1.0 (13.09.2026) его чуть
# не забыли: скрипт печатал список файлов выпуска, исходников в нём не было, и
# заметить пропажу можно было только сверкой с прошлым выпуском.
#
# Берётся из ТЕГА, а не из рабочего дерева: выложенные исходники обязаны быть
# тем, что лежит в истории, а не тем, что случайно осталось несохранённым.
$teg = "v$Versiya"
& git -C $koren rev-parse --verify --quiet "refs/tags/$teg" | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host "БЕЗ ИСХОДНИКОВ: тега $teg ещё нет. Поставить его и запустить выпуск заново." -ForegroundColor Yellow
} else {
    $ish = Join-Path $vypusk "Affory-$Versiya-sources.zip"
    Remove-Item $ish -Force -ErrorAction SilentlyContinue
    & git -C $koren archive --format=zip "--prefix=affory-$Versiya/" -o $ish $teg
    if ($LASTEXITCODE -ne 0) { throw 'архив исходников не собрался' }
    $hi = (Get-FileHash $ish -Algorithm SHA256).Hash.ToLower()
    [IO.File]::WriteAllText("$ish.sha256", "$hi  Affory-$Versiya-sources.zip`n")
}

Write-Host ''
Write-Host "=== Выпуск $Versiya ===" -ForegroundColor Cyan
Get-ChildItem $vypusk | ForEach-Object { Write-Host ("  {0,-34} {1,10:N0} байт" -f $_.Name, $_.Length) }
