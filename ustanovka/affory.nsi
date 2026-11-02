; Установщик Affory (NSIS 3, Unicode). Собирается ustanovka\sobrat-reliz.ps1,
; который кладёт бинари в ustanovka\sborka\ и передаёт версию через /DVERSIYA.
;
; Установщик нарочно тонкий: всё, что требует знаний о службе (каталог данных
; с разорванным наследованием, отпечатки, ключ Run, аварийный лист, снятие с
; вопросом про ключи), делает сама служба подкомандами install и uninstall
; (§4.4 спеки). Здесь только файлы, реестр «Программы и компоненты» и ярлыки.

Unicode true
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"

!ifndef VERSIYA
  !define VERSIYA "0.0.0"
!endif
!ifndef SBORKA
  !define SBORKA "sborka"
!endif

; Деинсталлятор пишет сам установщик директивой WriteUninstaller, до сборки
; его не существует, и подписать его снаружи нечем. Единственный способ это
; !uninstfinalize: NSIS зовёт команду на готовый Uninstall.exe. Путь к скрипту
; приходит через /DPODPISAT: относительный тут считается от каталога NSIS, а не нашего. Отпечаток не
; секрет, в отличие от пароля, поэтому его можно передать аргументом.
!ifdef OTPECHATOK
  !ifndef SIGN_POWERSHELL
    !error "Signing requires the PowerShell executable selected by sobrat-reliz.ps1"
  !endif
  !uninstfinalize '"${SIGN_POWERSHELL}" -NoProfile -ExecutionPolicy Bypass -File "${PODPISAT}" -Fayly "%1" -Otpechatok ${OTPECHATOK}' = 0
!endif

Name "Affory ${VERSIYA}"
OutFile "vypusk\Affory-${VERSIYA}-setup.exe"
InstallDir "$PROGRAMFILES64\Affory"
RequestExecutionLevel admin
SetCompressor /SOLID lzma
BrandingText "Affory ${VERSIYA}"

!define MUI_ICON "..\cmd\affory-ui\ikonki\affory.ico"
!define MUI_UNICON "..\cmd\affory-ui\ikonki\affory.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Открыть Affory"
!define MUI_FINISHPAGE_RUN_FUNCTION OtkrytOkno

!insertmacro MUI_PAGE_WELCOME
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE KatalogVybran
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Russian"

!define KLYUCH_UDALENIYA "Software\Microsoft\Windows\CurrentVersion\Uninstall\Affory"

; Имя обязано совпадать с cmd\affory-svc\vyvod.go (imyaFaylaPrichiny).
!define FAYL_PRICHINY "ustanovka-prichina.txt"

; Причина отказа приходит ФАЙЛОМ рядом с бинарём, а не только трубой.
;
; Трубу nsExec декодирует кодовой страницей ANSI машины. На нерусской Windows
; (проверено на 1252) кириллица в ней превращается в вопросы ещё до показа, а
; на русской она читается, но живёт ровно до закрытия окна. Файл пишется в
; UTF-16LE, и его кодировка не зависит ни от локали, ни от настроек.
;
; 21.09.2026 человек увидел «Служба Affory не установилась (код 1)» и ссылку на
; sluzhba.log, которого при неудачной установке не существует. Причина при этом
; была названа полностью: программа стояла не в C:\Program Files.
!macro PRICHINA put vyhod
  StrCpy ${vyhod} ""
  ClearErrors
  FileOpen $R9 "${put}" r
  ${IfNot} ${Errors}
    FileReadUTF16LE $R9 ${vyhod}
    FileClose $R9
  ${EndIf}
  Delete "${put}"
  ${If} ${vyhod} == ""
    StrCpy ${vyhod} "Подробности в C:\ProgramData\Affory\log\ustanovka.log"
  ${EndIf}
!macroend

; Лежит ли $INSTDIR внутри каталога $R1 или совпадает с ним. Ответ в $R0: "1"
; или "". Сравнение без учёта регистра, как в файловой системе.
Function VnutriKataloga
  StrCpy $R0 ""
  StrLen $R2 $R1
  StrCpy $R3 $INSTDIR $R2
  ${If} $R3 == $R1
    StrCpy $R3 $INSTDIR 1 $R2
    ${If} $R3 == ""
    ${OrIf} $R3 == "\"
      StrCpy $R0 "1"
    ${EndIf}
  ${EndIf}
FunctionEnd

; Каталог программы всегда отдельный и называется Affory (пункт К1 аудита 1.6.1).
;
; Путь можно вписать руками или передать /D=, а суффикс \Affory NSIS дописывает
; только через «Обзор». Affory, поставленная прямо в D:\Games, жила в общем
; каталоге: удаление до 1.6.2 стирало его целиком, а права такого каталога не
; закрыть, не отняв у человека всё остальное в нём. Поэтому к пути, который
; кончается не на Affory, дописывается \Affory: корень диска, Program Files и
; любой общий каталог становятся родителем, а не самим каталогом программы.
;
; Внутрь Windows и внутрь профилей не ставится вовсе. Папку в профиле её хозяин
; может переименовать и положить на её место свою, а служба запускает
; affory-svc.exe из каталога программы от SYSTEM.
;
; Каталог, где Affory уже стоит, остаётся как есть: перенос молча оставил бы
; старую копию с её Uninstall.exe, а тот снимает общую для обеих службу.
;
; Отказ приходит текстом в $R0, пустой $R0 значит каталог годен.
Function SvoyKatalog
  StrCpy $R0 ""
  ${If} ${FileExists} "$INSTDIR\affory-svc.exe"
    Return
  ${EndIf}
  StrCpy $R1 $INSTDIR 1 -1
  ${If} $R1 == "\"
    StrCpy $INSTDIR $INSTDIR -1
  ${EndIf}
  StrCpy $R1 $INSTDIR "" -7
  ${If} $R1 != "\Affory"
    StrCpy $INSTDIR "$INSTDIR\Affory"
  ${EndIf}
  ; Длинный путь (1.10.0). За 260 знаками часть Windows перестаёт находить
  ; файлы, если длинные пути не включены в реестре, а по умолчанию они
  ; выключены. 200 оставляют запас на имена файлов внутри.
  StrLen $R1 $INSTDIR
  ${If} $R1 > 200
    StrCpy $R0 "Путь к папке слишком длинный ($R1 знаков, можно до 200): с таким Windows не всегда находит файлы программы. Выбери папку покороче, например C:\Program Files."
    Return
  ${EndIf}
  StrCpy $R1 $WINDIR
  Call VnutriKataloga
  ${If} $R0 == "1"
    StrCpy $R0 "Внутрь папки Windows Affory не ставится. Выбери другую папку, например C:\Program Files."
    Return
  ${EndIf}
  ReadRegStr $R1 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList" "ProfilesDirectory"
  ExpandEnvStrings $R1 $R1
  ${If} $R1 != ""
    Call VnutriKataloga
    ${If} $R0 == "1"
      StrCpy $R0 "Внутрь папки пользователей ($R1) Affory не ставится. Выбери другую папку, например C:\Program Files."
      Return
    ${EndIf}
  ${EndIf}
FunctionEnd

; Выход со страницы каталога: тот же разбор, что и для /D=.
Function KatalogVybran
  Call SvoyKatalog
  ${If} $R0 != ""
    MessageBox MB_ICONEXCLAMATION "$R0"
    Abort
  ${EndIf}
FunctionEnd

; Окно открывается от имени вошедшего человека. Запущенное прямо из установщика,
; оно унаследовало бы его права администратора.
Function OtkrytOkno
  Exec '"$WINDIR\explorer.exe" "$INSTDIR\affory-ui.exe"'
FunctionEnd

!define GUID_WEBVIEW2 "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

; Стоит ли WebView2, по способу из документации Microsoft: у клиента Evergreen
; есть значение pv, непустое и не 0.0.0.0. Машинная установка лежит в
; 32-разрядной ветке реестра, но смотрятся обе ветки и установка пользователя.
; Ответ в $R0: "1" или "".
Function WebView2Est
  StrCpy $R0 ""
  SetRegView 32
  ReadRegStr $R1 HKLM "SOFTWARE\Microsoft\EdgeUpdate\Clients\${GUID_WEBVIEW2}" "pv"
  SetRegView 64
  Call WebView2Versiya
  ReadRegStr $R1 HKLM "SOFTWARE\Microsoft\EdgeUpdate\Clients\${GUID_WEBVIEW2}" "pv"
  Call WebView2Versiya
  ReadRegStr $R1 HKCU "Software\Microsoft\EdgeUpdate\Clients\${GUID_WEBVIEW2}" "pv"
  Call WebView2Versiya
FunctionEnd

Function WebView2Versiya
  ${If} $R1 != ""
  ${AndIf} $R1 != "0.0.0.0"
    StrCpy $R0 "1"
  ${EndIf}
FunctionEnd

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "Affory работает только на 64-разрядной Windows." /SD IDOK
    Abort
  ${EndIf}
  ; Windows 10 и новее (1.10.0): WebView2, фильтры WFP и манифесты программы
  ; рассчитаны на неё. Номер сборки из реестра, а не из GetVersionEx: тот
  ; отвечает по манифесту установщика. Нечитаемый номер установку не держит.
  ReadRegStr $R0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"
  ${If} $R0 != ""
  ${AndIf} $R0 < 10240
    MessageBox MB_ICONSTOP "Affory работает на Windows 10 и 11, а здесь более старая Windows (сборка $R0)." /SD IDOK
    Abort
  ${EndIf}
  SetRegView 64
  ; Ярлыки для всех пользователей: установщик и так требует администратора.
  SetShellVarContext all
  ; Обновление встаёт туда, где программа уже стоит, а не в каталог по
  ; умолчанию рядом со старой копией. InstallDirRegKey здесь не годится: он
  ; читает реестр до .onInit, в 32-разрядной ветке, а ключ удаления пишется в
  ; 64-разрядную. /D= главнее прежнего места: с ним $INSTDIR уже не умолчание.
  ${If} $INSTDIR == "$PROGRAMFILES64\Affory"
    ReadRegStr $R0 HKLM "${KLYUCH_UDALENIYA}" "InstallLocation"
    ${If} $R0 != ""
    ${AndIf} ${FileExists} "$R0\affory-svc.exe"
      StrCpy $INSTDIR $R0
    ${EndIf}
  ${EndIf}
  ; В обычном режиме негодный каталог увидит страница выбора, в тихом
  ; спросить некого.
  Call SvoyKatalog
  ${If} $R0 != ""
  ${AndIf} ${Silent}
    SetErrorLevel 2
    Abort
  ${EndIf}
FunctionEnd

; prepare-install останавливает службу, и прерванная после этого установка
; оставляла машину без VPN до перезагрузки. Запускается та служба, какая есть.
Function .onInstFailed
  nsExec::Exec '"$SYSDIR\sc.exe" query AfforySvc'
  Pop $0
  ${If} $0 == 0
    DetailPrint "Запуск службы AfforySvc после прерванной установки"
    nsExec::Exec '"$SYSDIR\sc.exe" start AfforySvc'
    Pop $0
    ; 1056: служба уже работает.
    ${If} $0 != 0
    ${AndIf} $0 != 1056
      DetailPrint "Служба AfforySvc не запустилась (код $0)"
    ${EndIf}
  ${EndIf}
FunctionEnd

Section "Affory" SEC_AFFORY
  SectionIn RO
  SetOutPath "$INSTDIR"
  ; Установка поверх стоящей версии. Запущенная служба держит affory-svc.exe
  ; и sing-box.exe, окно держит affory-ui.exe: File на занятом файле падает с
  ; «Невозможно записать» (так упало на хосте 03.09.2026). Сначала остановить,
  ; потом копировать. net stop синхронный и ждёт настоящей остановки; на машине
  ; без службы возвращает ошибку, и это не ошибка установки. Вывод обоих в
  ; журнал не идёт: строка «process not found» на чистой машине читается как
  ; поломка.
  InitPluginsDir
  File /oname=$PLUGINSDIR\affory-svc-setup.exe "${SBORKA}\affory-svc.exe"
  DetailPrint "Остановка Affory и восстановление её сетевых настроек"
  Delete "$PLUGINSDIR\${FAYL_PRICHINY}"
  nsExec::ExecToLog '"$PLUGINSDIR\affory-svc-setup.exe" prepare-install "$INSTDIR"'
  Pop $0
  ${If} $0 != 0
    !insertmacro PRICHINA "$PLUGINSDIR\${FAYL_PRICHINY}" $1
    MessageBox MB_ICONSTOP "Не удалось подготовить Affory к установке (код $0).$\r$\n$\r$\n$1$\r$\n$\r$\nУстановленные файлы оставлены на месте." /SD IDOK
    Abort
  ${EndIf}
  ; Окно и дерево WebView2 закрывает сама prepare-install: она ищет процессы ПО
  ; ПУТИ ОБРАЗА внутри $INSTDIR и дожидается, пока файлы отпустят. Прежде здесь
  ; стоял `taskkill /IM affory-ui.exe /F /T` плюс `Sleep 1000`: первый убивал
  ; любой процесс с таким именем, включая чужой и вторую копию Affory из другого
  ; каталога, а вторая была ставкой на то, что антивирус и индексатор отпустят
  ; файл за секунду. Проиграв её, установка падала на первом же File.
  File "${SBORKA}\affory-svc.exe"
  File "${SBORKA}\affory-cli.exe"
  File "${SBORKA}\affory-ui.exe"
  File "${SBORKA}\sing-box.exe"
  ; Условие GPL: рядом с ядром обязан лежать текст лицензии и указание, где
  ; взять его исходники. Ядро это сборка sing-box, а не наш код.
  File "GPL-3.0.txt"
  File "YADRO-ISHODNIKI.txt"
  ; Программу сервера «Своего VPN» клиент кладёт на сервер человека, и она
  ; тоже сборка на sing-box.
  File "UZEL-ISHODNIKI.txt"
  File /oname=LICENSE.opencck.txt "..\internal\katalog\LICENSE.opencck"
  File /oname=LICENSE.simple-icons.txt "..\cmd\affory-ui\frontend\src\assets\services\LICENSE.simple-icons.txt"
  File /oname=OFL-Inter.txt "..\cmd\affory-ui\frontend\src\shrifty\OFL-Inter.txt"
  ; Установка поверх прежней версии: Manrope из дерева ушёл, но его лицензия
  ; лежит рядом с программой у всех, кто ставил 1.1.x.
  Delete "$INSTDIR\OFL-Manrope.txt"

  ; Деинсталлятор и запись в «Программах» появляются ДО установки службы. Если
  ; install упадёт на первой установке, файлы уже лежат, и без этих двух
  ; вещей штатно снять их было бы нечем.
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "DisplayName" "Affory"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "DisplayVersion" "${VERSIYA}"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "DisplayIcon" "$INSTDIR\affory-ui.exe"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKLM "${KLYUCH_UDALENIYA}" "NoModify" 1
  WriteRegDWORD HKLM "${KLYUCH_UDALENIYA}" "NoRepair" 1

  ; Служба ставит себя сама: каталог данных, отпечатки, ключ Run, аварийный
  ; лист, запуск. Ставится поверх существующей (обновление), режим не снимается.
  DetailPrint "Установка службы AfforySvc"
  Delete "$INSTDIR\${FAYL_PRICHINY}"
  nsExec::ExecToLog '"$INSTDIR\affory-svc.exe" install'
  Pop $0
  ${If} $0 != 0
    !insertmacro PRICHINA "$INSTDIR\${FAYL_PRICHINY}" $1
    MessageBox MB_ICONSTOP "Служба Affory не установилась (код $0).$\r$\n$\r$\n$1" /SD IDOK
    Abort
  ${EndIf}

  ; WebView2 рисует окно. На обычных Windows 10 и 11 он уже стоит, а на LTSC и
  ; урезанных сборках его может не быть, и окно выходило чёрным (пункт В8
  ; аудита 1.6.1). Вшитый загрузчик Microsoft сам скачивает рантайм. Без сети он
  ; не встанет, но установку это не отменяет: окно при запуске скажет, чего ему
  ; не хватает, и даст ссылку.
  Call WebView2Est
  ${If} $R0 == ""
    DetailPrint "Установка компонента Microsoft Edge WebView2"
    File /oname=$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe "${SBORKA}\MicrosoftEdgeWebview2Setup.exe"
    ClearErrors
    ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install' $0
    ${If} ${Errors}
    ${OrIf} $0 != 0
      DetailPrint "WebView2 не установился (код $0): окно Affory не откроется, пока его нет"
    ${EndIf}
    ; Код загрузчика не говорит, встал ли рантайм: он отвечает и за скачивание.
    ; Сверка тем же способом, что и до установки (1.10.0). Строчку в журнале
    ; установки человек не читает, поэтому говорится сообщением.
    Call WebView2Est
    ${If} $R0 == ""
      MessageBox MB_YESNO|MB_ICONEXCLAMATION "Affory установлена, но для её окна нужен компонент Microsoft Edge WebView2, а он не установился (обычно нет сети).$\r$\n$\r$\nОткрыть страницу загрузки на сайте Microsoft?" /SD IDNO IDNO webview2_bez_ssylki
        ; Через explorer, как и окно: браузер не наследует права установщика.
        Exec '"$WINDIR\explorer.exe" "https://go.microsoft.com/fwlink/p/?LinkId=2124703"'
      webview2_bez_ssylki:
    ${EndIf}
  ${EndIf}

  CreateDirectory "$SMPROGRAMS\Affory"
  CreateShortcut "$SMPROGRAMS\Affory\Affory.lnk" "$INSTDIR\affory-ui.exe"
  CreateShortcut "$SMPROGRAMS\Affory\Удалить Affory.lnk" "$INSTDIR\Uninstall.exe"
SectionEnd

Function un.onInit
  SetRegView 64
  SetShellVarContext all
FunctionEnd

Section "Uninstall"
  ; Окно закрывает сама `uninstall`: она ищет процессы по пути образа внутри
  ; каталога программы. Прежде здесь стоял `taskkill /IM affory-ui.exe /F`,
  ; который бил по имени и мог снять чужой процесс-однофамильца.

  ; Вопрос про ключи задаёт тот, кто снимает, а не служба: в тихом режиме (/S)
  ; ключи остаются, стирание необратимо и по умолчанию не делается.
  StrCpy $1 ""
  ${IfNot} ${Silent}
    MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Стереть ключи и настройки Affory (C:\ProgramData\Affory)?$\r$\nЭто необратимо. «Нет» оставит их для следующей установки." IDNO +2
      StrCpy $1 " --steret-klyuchi"
  ${EndIf}
  DetailPrint "Снятие службы AfforySvc"
  Delete "$INSTDIR\${FAYL_PRICHINY}"
  nsExec::ExecToLog '"$INSTDIR\affory-svc.exe" uninstall$1'
  Pop $0
  ${If} $0 != 0
    !insertmacro PRICHINA "$INSTDIR\${FAYL_PRICHINY}" $2
    MessageBox MB_ICONSTOP "Не удалось остановить Affory и восстановить сеть (код $0).$\r$\n$\r$\n$2$\r$\n$\r$\nФайлы оставлены для повторной попытки." /SD IDOK
    Abort
  ${EndIf}

  Delete "$INSTDIR\affory-svc.exe"
  Delete "$INSTDIR\affory-cli.exe"
  Delete "$INSTDIR\affory-ui.exe"
  Delete "$INSTDIR\sing-box.exe"
  Delete "$INSTDIR\GPL-3.0.txt"
  Delete "$INSTDIR\YADRO-ISHODNIKI.txt"
  Delete "$INSTDIR\UZEL-ISHODNIKI.txt"
  Delete "$INSTDIR\LICENSE.opencck.txt"
  Delete "$INSTDIR\LICENSE.simple-icons.txt"
  Delete "$INSTDIR\OFL-Inter.txt"
  ; Manrope ушёл вместе с редизайном 16.09.2026: старая установка держит его
  ; лицензию рядом с программой, и обновление обязано её убрать.
  Delete "$INSTDIR\OFL-Manrope.txt"
  Delete "$INSTDIR\ESLI-NET-INTERNETA.txt"
  Delete "$INSTDIR\*.ubrat"
  Delete "$INSTDIR\*.chast"
  Delete "$INSTDIR\*.proba"
  Delete "$INSTDIR\${FAYL_PRICHINY}"
  RMDir /r "$INSTDIR\novaya"
  RMDir /r "$INSTDIR\predydushchaya"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\Affory\Affory.lnk"
  Delete "$SMPROGRAMS\Affory\Удалить Affory.lnk"
  RMDir "$SMPROGRAMS\Affory"
  DeleteRegKey HKLM "${KLYUCH_UDALENIYA}"
SectionEnd
