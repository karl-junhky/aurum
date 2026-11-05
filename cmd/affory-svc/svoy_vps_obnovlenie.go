package main

import (
	"context"
	"errors"
	"log"
	"maps"
	"slices"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

// Обновление программы своего сервера (З2). Программа та же, что ставит
// установка этой сборки клиента: сверенная копия с выпуска. Новая встаёт
// под таймером отката на сервере и подтверждается, только когда поднялась и
// провезла ключи из сети человека.

const vidObnovlenie = "obnovlenie"

// bezopasnayaVersiyaUzla: программа сервера старше этой несёт уязвимость,
// исправленную в новой. Тогда о ней тревожит и общий статус, а не только
// карточка сервера. Поднимается выпуском, где такое исправление есть.
const bezopasnayaVersiyaUzla = "0.6.0"

// ObnovlenieVps это итог обновления для карточки: номера программы до и
// после. Uzhe - эта программа уже стояла. NeProshli - виды ключей, что
// провозили из сети человека до обновления, а после него нет.
type ObnovlenieVps struct {
	Bylo      string   `json:"bylo,omitempty"`
	Versiya   string   `json:"versiya"`
	Uzhe      bool     `json:"uzhe,omitempty"`
	NeProshli []string `json:"ne_proshli,omitempty"`
}

// noveeNaServere отдаёт номер программы, которую ставит эта сборка, если
// он новее versiya с сервера; иначе пусто. Номер с сервера не известен -
// тоже пусто: сравнивать не с чем, а проверка его узнает.
func (s *Sluzhba) noveeNaServere(versiya string) string {
	u, err := s.vshityyUzel()
	if err != nil || versiya == "" || !novee(u.Versiya, versiya) {
		return ""
	}
	return u.Versiya
}

// svoyVpsObnovit ставит на свой сервер программу из этой сборки клиента.
func (s *Sluzhba) svoyVpsObnovit(k protokol.Kadr) protokol.Kadr {
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	u, err := s.vshityyUzel()
	if err != nil {
		return otkazIz(k, protokol.KodVpsObnovlenie, err)
	}
	// Та же версия идёт дальше: неподтверждённую с прошлого раза обновление
	// доводит до конца, а подтверждённая отвечает «уже стоит».
	if z.Versiya != "" && novee(z.Versiya, u.Versiya) {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "на сервере уже программа "+z.Versiya+", новее в этой версии Affory нет")
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidObnovlenie, func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag, h.Server = shagZagruzka, z.Id })
		prog, err := s.programmaUzla(ctx)
		if err != nil {
			return err
		}
		defer prog.Close()
		pom := pomehiKompyutera(s.obnovitPomehi())
		o := upravlenie.Obnovlenie{
			Puti: s.putiVps(), Vhod: zapros, Programma: prog, Versiya: u.Versiya, Zapusk: s.zapuskProverki,
			Hod: func(h upravlenie.Hod) { s.pravitHod(zad, func(x *HodVps) { primenitHod(x, h, pom) }) },
		}
		o.Klyuchi = s.klyuchiProshedshie(z)
		if len(o.Klyuchi) > 0 {
			a, err := s.fizicheskiyAdapter()
			if err != nil {
				return err
			}
			o.Proverka = proverka.Opcii{Adapter: a.Imya}
		}
		it, err := s.vpsObnovit(ctx, o)
		if err != nil {
			var no *upravlenie.NeObnovilsya
			if errors.As(err, &no) && no.Vozvrat != nil {
				log.Printf("свой сервер %s: прежняя программа сразу не вернулась, её вернёт таймер: %v", z.Id, no.Vozvrat)
				// Отменённое дело причины не показывает, а сервер ещё до 10
				// минут работает непроверенной программой.
				if ctx.Err() != nil {
					s.pravitHod(zad, func(h *HodVps) {
						h.Preduprezhdeniya = append(h.Preduprezhdeniya, OtkazVps{Kod: protokol.KodVpsObnovlenie,
							Tekst: "сразу вернуть прежнюю программу не вышло, сервер вернёт её сам через 10 минут"})
					})
				}
			}
			return err
		}
		neProshli := neProvezli(it.Proverki)
		// Обновление это визит, как проверка: прежняя проверка видела старую
		// программу, и карточка не должна показывать её номер. Программа,
		// что уже стояла, не проверялась, и прошлый отказ проверки остаётся.
		seychas := s.seychas().UTC()
		zapisana := s.otmetitProverku(z.Id, func(x *SvoyServer) {
			x.Versiya = it.Versiya
			if !it.Uzhe {
				x.Proveren, x.ProverkaOtkaz = seychas, ""
				dobavitNeIzSeti(x, neProshli)
			}
		})
		if zapisana != nil {
			log.Printf("свой сервер %s: версия программы не записана: %v", z.Id, zapisana)
		}
		s.pravitHod(zad, func(h *HodVps) {
			h.Put = it.Vhod.Put
			h.Obnovlenie = &ObnovlenieVps{Bylo: it.Bylo, Versiya: it.Versiya, Uzhe: it.Uzhe,
				NeProshli: slices.Sorted(maps.Keys(neProshli))}
			if zapisana != nil {
				h.Preduprezhdeniya = append(h.Preduprezhdeniya, OtkazVps{Kod: kodSohraneniya(zapisana), Tekst: sboi.DlyaCheloveka(zapisana)})
			}
		})
		return nil
	})
}

// klyuchiProshedshie это ключи сервера z тех видов, что провозили из сети
// человека: после обновления хоть один из них обязан провезти. Вид, который
// не проходил и до него, отката не вызывает. Набор не прочитан - ключей нет,
// и о новой программе судит она сама.
func (s *Sluzhba) klyuchiProshedshie(z SvoyServer) []protokol.Server {
	n, err := s.nabor()
	if err != nil {
		log.Printf("свой сервер %s: ключи для проверки обновления не прочитаны: %v", z.Id, err)
		return nil
	}
	var k []protokol.Server
	for _, srv := range n.Servery {
		if slices.Contains(z.Servery, srv.Id) && !slices.Contains(z.NeIzSeti, srv.Transport) {
			k = append(k, srv)
		}
	}
	return k
}

// neProvezli это виды из итогов проверки, у которых ни один ключ не провёз,
// с причиной для записи о сервере.
func neProvezli(itogi []proverka.Itog) map[string]string {
	vezut := map[string]bool{}
	for _, x := range itogi {
		if x.Vid == proverka.Vezyot {
			vezut[x.Server.Transport] = true
		}
	}
	ne := map[string]string{}
	for _, x := range itogi {
		t := x.Server.Transport
		if vezut[t] || ne[t] != "" {
			continue
		}
		ne[t] = "после обновления программы сервера ключ " + imyaVida(t) + " не провёз проверку из твоей сети"
		if x.Otkaz != "" {
			ne[t] += ": " + x.Otkaz
		}
	}
	return ne
}

// dobavitNeIzSeti отмечает в записи виды, что перестали провозить из сети,
// как это делает установка: общая причина и, когда видов больше одного,
// своя у каждого.
func dobavitNeIzSeti(x *SvoyServer, novye map[string]string) {
	if len(novye) == 0 {
		return
	}
	poVidam := maps.Clone(x.SetPoVidam)
	if poVidam == nil {
		poVidam = map[string]string{}
	}
	// Один вид держал причину только общей строкой.
	if len(x.NeIzSeti) == 1 && poVidam[x.NeIzSeti[0]] == "" {
		poVidam[x.NeIzSeti[0]] = x.SetTekst
	}
	for _, t := range slices.Sorted(maps.Keys(novye)) {
		if !slices.Contains(x.NeIzSeti, t) {
			x.NeIzSeti = append(x.NeIzSeti, t)
		}
		poVidam[t] = novye[t]
	}
	x.SetTekst, x.SetPoVidam = tekstVidov(x.NeIzSeti, poVidam), poVidam
	if len(x.NeIzSeti) < 2 {
		x.SetPoVidam = nil
	}
}

// otkazObnovleniya переводит отказ обновления после замены; ok false - отказ
// не про это.
func otkazObnovleniya(err error) (OtkazVps, bool) {
	var (
		no *upravlenie.NeObnovilsya
		np *upravlenie.NePodtverzhdeno
	)
	switch {
	case errors.As(err, &no):
		t := "программа сервера не обновилась: " + prichinaObnovleniya(no.Prichina)
		if no.Vozvrat != nil {
			t += ". Сразу вернуть прежнюю не вышло, сервер вернёт её сам через 10 минут"
		} else {
			t += ". Прежняя программа возвращена"
		}
		return OtkazVps{Kod: protokol.KodVpsObnovlenie, Tekst: t}, true
	case errors.As(err, &np):
		return OtkazVps{Kod: protokol.KodVpsObnovlenie,
			Tekst: "новая программа сервера заработала, но подтверждение до сервера не дошло, и через 10 минут он вернёт прежнюю. Повтори обновление"}, true
	case errors.Is(err, upravlenie.ErrOtkatilsyaSam):
		return OtkazVps{Kod: protokol.KodVpsObnovlenie,
			Tekst: "сервер вернул прежнюю программу раньше, чем пришло подтверждение: повтори обновление"}, true
	}
	return OtkazVps{}, false
}

func prichinaObnovleniya(err error) string {
	var np *upravlenie.NePodnyalas
	switch {
	case errors.Is(err, upravlenie.ErrKlyuchiNeVezut):
		return "с ней ни один ключ не провёз из твоей сети"
	case errors.As(err, &np):
		return "новая не поднялась за 2 минуты (" + sboi.DlyaCheloveka(np.Posledneye) + ")"
	case errors.Is(err, upravlenie.ErrZamenaNeVstala):
		return "новая так и не встала на место"
	case errors.Is(err, upravlenie.ErrProverkaDolgo):
		return "проверка ключей из твоей сети не уложилась в срок"
	}
	return sboi.DlyaCheloveka(err)
}
