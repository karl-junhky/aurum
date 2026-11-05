package server

import (
	"context"
	_ "embed"
	"io"
	"time"
)

// Обновление программы на поставленном сервере (З2): новая кладётся рядом,
// проверяется на живом пакете, прежняя остаётся копией под таймером отката,
// перезапуск идёт очередью systemd без этого входа. Подтверждает или
// откатывает клиент новым входом, после своей проверки.

//go:embed skripty/obnovlenie.sh
var skriptObnovleniya string

// SrokOtkata это срок таймера отката в obnovlenie.sh (srok_otkata): за него
// клиент обязан проверить новую программу и подтвердить её.
const SrokOtkata = 10 * time.Minute

// ProgrammaNaMeste это программа сервера на диске глазами obnovlenie.sh.
type ProgrammaNaMeste struct {
	Sha256 string
	// Versiya это ответ vpn-uzel versiya; пуст, если программа не ответила.
	Versiya string
	// OtkatIdyot: программа на месте не подтверждена. Таймер отката идёт или
	// пропал с перезагрузкой сервера, а копия прежней ещё лежит.
	OtkatIdyot bool
}

// Zamena это итог ZamenitProgrammu.
type Zamena struct {
	// Bylo это программа на месте до замены.
	Bylo ProgrammaNaMeste
	// Uzhe: на месте уже эта программа, и она подтверждена: менять нечего.
	Uzhe bool
	// Versiya это версия новой программы, когда её поставили (или она уже
	// стоит неподтверждённой).
	Versiya string
}

// ObryvZameny: вход оборвался, пока шла замена. Перезапуск программы рвёт и
// SSH, если VPN человека идёт через этот же сервер, поэтому обрыв ещё не
// отказ: что на месте, говорит новый вход (ProgrammaNaMesteSeychas).
type ObryvZameny struct{ Err error }

func (o *ObryvZameny) Error() string {
	return "вход оборвался во время замены программы: " + o.Err.Error()
}
func (o *ObryvZameny) Unwrap() error { return o.Err }

// ProgrammaNaMesteSeychas читает, какая программа на диске и идёт ли таймер
// отката.
func ProgrammaNaMesteSeychas(ctx context.Context, m Mashina) (p ProgrammaNaMeste, err error) {
	defer zashchita("ProgrammaNaMesteSeychas", &err)
	return programmaIz(ctx, m, "sostoyanie")
}

func programmaIz(ctx context.Context, m Mashina, rezhim string) (p ProgrammaNaMeste, err error) {
	it, err := m.Skript(ctx, skriptObnovleniya, rezhim)
	if err != nil {
		return p, err
	}
	if it.Kod != 0 {
		return p, oshibkaSkripta("obnovlenie", it)
	}
	stroki(it.Vyvod, func(k, z string) {
		switch k {
		case "ustanovleno":
			p.Sha256 = chisto(z)
		case "versiya":
			p.Versiya = chisto(z)
		case "otkat":
			p.OtkatIdyot = z == "idyot"
		}
	})
	if p.Sha256 == "" {
		return p, &Oshibka{Shag: "obnovlenie", Tekst: "сервер не назвал sha256 программы"}
	}
	return p, nil
}

// ZamenitProgrammu ставит programma на место прежней. Та же программа уже
// стоит и подтверждена - Uzhe. Иначе файл уходит в каталог загрузки и
// obnovlenie.sh zamenit его проверяет, заводит таймер и ставит перезапуск в
// очередь. Та же, но неподтверждённая с прошлого раза, тоже идёт заменой:
// таймер прошлого раза уже отсчитал своё или пропал с перезагрузкой, а срок
// проверки клиент считает от этой замены. Обрыв входа на замене -
// *ObryvZameny.
func ZamenitProgrammu(ctx context.Context, m Mashina, programma io.ReadSeeker) (z Zamena, err error) {
	defer zashchita("ZamenitProgrammu", &err)
	hesh, err := sha256Chitatelya(programma)
	if err != nil {
		return z, err
	}
	if z.Bylo, err = programmaIz(ctx, m, "nachalo"); err != nil {
		return z, err
	}
	if z.Bylo.Sha256 == hesh && !z.Bylo.OtkatIdyot {
		z.Uzhe, z.Versiya = true, z.Bylo.Versiya
		return z, nil
	}
	if err := m.OtpravitFayl(ctx, programma, katalogZagruzki+"/vpn-uzel", 0o700); err != nil {
		return z, err
	}
	it, err := m.Skript(ctx, skriptObnovleniya, "zamenit")
	if err != nil {
		return z, &ObryvZameny{err}
	}
	gotovo := false
	stroki(it.Vyvod, func(k, v string) {
		switch k {
		case "versiya":
			z.Versiya = chisto(v)
		case "gotovo":
			gotovo = true
		}
	})
	if it.Kod != 0 || !gotovo {
		return z, oshibkaSkripta("obnovlenie", it)
	}
	return z, nil
}

// PodtverditObnovlenie снимает таймер отката и копию прежней программы и
// отдаёт версию той, что на месте: таймер мог успеть вернуть прежнюю.
func PodtverditObnovlenie(ctx context.Context, m Mashina) (versiya string, err error) {
	defer zashchita("PodtverditObnovlenie", &err)
	return obnovlenieShag(ctx, m, "podtverdit")
}

// OtkatitObnovlenie возвращает прежнюю программу из копии, ставит
// перезапуск в очередь и снимает таймер. Отдаёт версию той, что на месте.
func OtkatitObnovlenie(ctx context.Context, m Mashina) (versiya string, err error) {
	defer zashchita("OtkatitObnovlenie", &err)
	return obnovlenieShag(ctx, m, "otkatit")
}

func obnovlenieShag(ctx context.Context, m Mashina, rezhim string) (string, error) {
	it, err := m.Skript(ctx, skriptObnovleniya, rezhim)
	if err != nil {
		return "", err
	}
	versiya, gotovo := "", false
	stroki(it.Vyvod, func(k, z string) {
		switch k {
		case "versiya":
			versiya = chisto(z)
		case "gotovo":
			gotovo = true
		}
	})
	if it.Kod != 0 || !gotovo {
		return "", oshibkaSkripta("obnovlenie", it)
	}
	return versiya, nil
}
