package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/domen"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

const domenTesta = "kot.example.net"

// itogDomena это итог дела, после которого сервер в режиме домена: маска и
// имя hy2 это домен, подписка с правилами по rfCherezVpn.
func itogDomena(t *testing.T, sek konfig.Sekrety, kh ssh.KlyuchHosta, rfCherezVpn bool) upravlenie.Itog {
	t.Helper()
	sek.Maska = domenTesta
	it := itogSoSekretami(t, sek, kh, 443, 8443)
	routing, err := routingDlya(rfCherezVpn)
	if err != nil {
		t.Fatal(err)
	}
	if it.Vhody.Sayt, err = konfig.NovyySayt(domenTesta, routing); err != nil {
		t.Fatal(err)
	}
	if it.Klyuchi, err = konfig.Klyuchi(sek, it.Vhody); err != nil {
		t.Fatal(err)
	}
	it.MaskaIstochnik, it.Versiya = upravlenie.MaskaDomen, "0.7.0"
	return it
}

// naDomene ставит сервер для подбора и переводит его запись в режим домена.
func naDomene(t *testing.T, s *Sluzhba, kh ssh.KlyuchHosta, rfCherezVpn bool) (string, konfig.Sekrety, upravlenie.Itog) {
	t.Helper()
	id, _, sek := postavitDlyaPodbora(t, s, kh)
	it := itogDomena(t, sek, kh, rfCherezVpn)
	if _, _, err := s.zapisatSvoy(id, osnovaZapisi{Imya: "Дача", Port: 2222, Polzovatel: "admin", Postavlen: postavlenDlyaTesta, ZapisEst: true}, it); err != nil {
		t.Fatal(err)
	}
	return id, sek, it
}

func TestSvoyVpsDomen(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _, sek := postavitDlyaPodbora(t, s, kh)
	// Российские сайты через VPN: в подписку едет профиль «всё через VPN».
	if err := s.pravitNabor(func(n *Nabor) error {
		z := &n.SvoiServery[0]
		z.RfCherezVpn, z.NeIzSeti, z.SetTekst = true, []string{"reality-tcp", "hy2"}, "оба не прошли"
		z.SetPoVidam = map[string]string{"reality-tcp": "reality глушат", "hy2": "hy2 режут"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var p upravlenie.SmenaRezhima
	s.vpsSmenitRezhim = func(_ context.Context, x upravlenie.SmenaRezhima) (upravlenie.Itog, error) {
		p = x
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagSertifikat})
		return itogDomena(t, sek, kh, true), nil
	}
	if otv := s.svoyVpsDomen(kadrVps(t, "svoyVpsDomen", map[string]string{"id": id, "domen": " https://Kot.Example.NET/ "})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Vid != vidDomen || h.Server != id || h.Domen == nil || h.Domen.Domen != domenTesta || h.Ustanovlen == nil {
		t.Fatalf("переход на домен: %+v", h)
	}
	vykl, err := routingDlya(true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Domen != domenTesta || p.Routing != vykl || p.Imya != "Дача" || p.Vhod.Port != 2222 {
		t.Fatalf("смена ушла с %+v", p)
	}
	zap := zapisPoId(t, s, id)
	if zap.Domen != domenTesta || !strings.HasPrefix(zap.Podpiska, "https://"+domenTesta+"/") || zap.Maska != domenTesta ||
		zap.MaskaIstochnik != upravlenie.MaskaDomen || !zap.RfCherezVpn || zap.Versiya != "0.7.0" {
		t.Fatalf("запись: %+v", zap)
	}
	// Порт hysteria2 тот же: его отметка о сети остаётся. Reality с новой
	// маской не проверялся.
	if !slices.Equal(zap.NeIzSeti, []string{"hy2"}) || zap.SetTekst != "hy2 режут" {
		t.Fatalf("отметки о сети: %v %q", zap.NeIzSeti, zap.SetTekst)
	}

	// Обратно к простым ключам: пустой домен, домен и ссылка уходят из записи.
	s.vpsSmenitRezhim = func(_ context.Context, x upravlenie.SmenaRezhima) (upravlenie.Itog, error) {
		p = x
		it := itogSoSekretami(t, sek, kh, 443, 8443)
		it.MaskaIstochnik = upravlenie.MaskaSosedi
		return it, nil
	}
	if otv := s.svoyVpsDomen(kadrVps(t, "svoyVpsDomen", map[string]string{"id": id, "domen": "  "})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo || h.Domen == nil || h.Domen.Domen != "" {
		t.Fatalf("к простым ключам: %+v", h)
	}
	if zap := zapisPoId(t, s, id); p.Domen != "" || zap.Domen != "" || zap.Podpiska != "" || zap.MaskaIstochnik != upravlenie.MaskaSosedi || !zap.RfCherezVpn {
		t.Fatalf("запись после простых ключей: %+v, смена %+v", zap, p)
	}
}

// «Поставить заново» на тот же домен оставляет ссылку подписки: телефоны
// заберут по ней новые ключи сами. На другой домен ссылка новая.
func TestSvoyVpsUstanovitZanovoNaDomene(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _, _ := naDomene(t, s, kh, false)
	put := konfig.PutPodpiski(zapisPoId(t, s, id).Podpiska)
	if put == "" {
		t.Fatal("в записи нет пути подписки")
	}
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) {
		return &programmaDlyaTesta{Reader: bytes.NewReader(nil)}, nil
	}
	var u upravlenie.Ustanovka
	s.vpsUstanovit = func(_ context.Context, x upravlenie.Ustanovka) (upravlenie.Itog, error) {
		u = x
		return upravlenie.Itog{}, errors.New("дальше не нужно")
	}
	// Новая машина на тот же домен (переезд) берёт путь у прежней.
	for _, c := range []struct {
		adres    string
		domen    string
		prezhniy bool
	}{{adresSvoego, domenTesta, true}, {adresSvoego, "pes.example.net", false}, {"203.0.113.20", domenTesta, true}} {
		u = upravlenie.Ustanovka{}
		poverh := ""
		if c.adres == adresSvoego {
			poverh = adresSvoego
		}
		z := zaprosUstanovkiVps{dostupVps: dostupVps{Adres: c.adres, Port: 2222, Polzovatel: "root", Parol: "x"},
			Pustoy: true, KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64(), Poverh: poverh, Domen: c.domen}
		if otv := s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z)); otv.Oshib != nil {
			t.Fatal(otv.Oshib)
		}
		dozhdatsyaVps(t, s)
		if u.Sayt == nil || u.Sayt.Domen != c.domen || (u.Sayt.Put == put) != c.prezhniy {
			t.Fatalf("%s: сайт %+v, прежний путь %v", c.domen, u.Sayt, c.prezhniy)
		}
	}
}

// Ссылка подписки сменилась не по просьбе (здесь дело домена вернуло другой
// путь): телефоны с прежней ничего не получат, и человек должен это знать.
func TestSvoyVpsPodpiskaSmenilas(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, sek, bylo := naDomene(t, s, kh, false)
	s.vpsSmenitRezhim = func(context.Context, upravlenie.SmenaRezhima) (upravlenie.Itog, error) {
		return itogDomena(t, sek, kh, false), nil
	}
	if otv := s.svoyVpsDomen(kadrVps(t, "svoyVpsDomen", map[string]string{"id": id, "domen": domenTesta})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if zap := zapisPoId(t, s, id); zap.Podpiska == bylo.Vhody.SsylkaPodpiski() {
		t.Fatal("ссылка в записи не сменилась, проверять нечего")
	}
	if h.Faza != fazaGotovo || len(h.Preduprezhdeniya) != 1 || h.Preduprezhdeniya[0].Kod != protokol.KodVpsPodpiskaSmenilas {
		t.Fatalf("предупреждения %+v", h.Preduprezhdeniya)
	}
}

// Домен переезжает на новый сервер: запись нового своей подписки не имела,
// а телефоны подписаны на ссылку прежнего. Её и надо сравнивать с новой.
func TestZapisatSvoyPereezdDomena(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _, _ := naDomene(t, s, kh, false)
	staraya := zapisPoId(t, s, id).Podpiska
	// Новая машина: свои секреты, ключи на свой адрес.
	novyy := netip.MustParseAddr("203.0.113.20")
	sek, err := konfig.NovyeSekrety(domenTesta)
	if err != nil {
		t.Fatal(err)
	}
	it := itogDomena(t, sek, kh, false)
	it.Adres, it.Vhody.Adres = novyy.String(), novyy.String()
	if it.Klyuchi, err = konfig.Klyuchi(sek, it.Vhody); err != nil {
		t.Fatal(err)
	}
	zad := &zadanieVps{snimok: HodVps{Vid: vidUstanovka}}
	if err := s.zapisatIPokazat(zad, idSvoego(novyy), osnovaZapisi{Imya: "Новый"}, it); err != nil {
		t.Fatal(err)
	}
	// Путь новый (itogDomena собрал свой сайт): телефоны с прежней ссылкой
	// должны узнать, что её надо сменить.
	if it.Vhody.SsylkaPodpiski() == staraya {
		t.Fatal("ссылка не сменилась, проверять нечего")
	}
	if p := zad.snimok.Preduprezhdeniya; len(p) != 1 || p[0].Kod != protokol.KodVpsPodpiskaSmenilas {
		t.Fatalf("предупреждения %+v", p)
	}
	if slices.ContainsFunc(zapisPoId(t, s, idSvoego(novyy)).Servery, func(x string) bool {
		return slices.Contains(zapisPoId(t, s, id).Servery, x)
	}) {
		t.Fatal("у двух серверов общие ключи")
	}
}

// Домен из карточки подключается к новой машине, а он уже у другого своего
// сервера: путь подписки берётся у того, и ссылка на телефонах та же.
func TestSvoyVpsDomenPereezd(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _, _ := postavitDlyaPodbora(t, s, kh)
	// Прежний сервер на домене, с другим адресом.
	staryy := netip.MustParseAddr("203.0.113.20")
	sek, err := konfig.NovyeSekrety(domenTesta)
	if err != nil {
		t.Fatal(err)
	}
	it := itogDomena(t, sek, kh, false)
	it.Adres, it.Vhody.Adres = staryy.String(), staryy.String()
	if it.Klyuchi, err = konfig.Klyuchi(sek, it.Vhody); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.zapisatSvoy(idSvoego(staryy), osnovaZapisi{Imya: "Прежний"}, it); err != nil {
		t.Fatal(err)
	}
	var p upravlenie.SmenaRezhima
	s.vpsSmenitRezhim = func(_ context.Context, x upravlenie.SmenaRezhima) (upravlenie.Itog, error) {
		p = x
		return upravlenie.Itog{}, errors.New("дальше не нужно")
	}
	if otv := s.svoyVpsDomen(kadrVps(t, "svoyVpsDomen", map[string]string{"id": id, "domen": domenTesta})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	dozhdatsyaVps(t, s)
	if p.Put == "" || p.Put != konfig.PutPodpiski(zapisPoId(t, s, idSvoego(staryy)).Podpiska) {
		t.Fatalf("путь %q", p.Put)
	}
}

// Предупреждение о сайте, о котором ничего не известно, не выдаёт себя за
// «сайт молчит» и не несёт технического текста по-английски.
func TestPreduprezhdenieSaytNeProveren(t *testing.T) {
	o := preduprezhdenieVps([]upravlenie.Preduprezhdenie{{Vid: upravlenie.SaytNeProveren, Domen: domenTesta, Otkaz: "ssh: unexpected EOF"}}, nil)
	if o.Kod != protokol.KodVpsSaytNeProveren || strings.Contains(o.Tekst, "EOF") || strings.Contains(o.Tekst, "не ответил") {
		t.Fatalf("%+v", o)
	}
}

// Установка на домен пошла с простыми ключами: домен из хода уходит.
func TestPrimenitHodBezDomena(t *testing.T) {
	x := HodVps{Domen: &DomenVps{Domen: domenTesta}}
	primenitHod(&x, upravlenie.Hod{Shag: upravlenie.ShagSertifikat}, nil)
	if x.Domen == nil {
		t.Fatal("обычный шаг снял домен")
	}
	primenitHod(&x, upravlenie.Hod{Shag: upravlenie.ShagSertifikat, BezDomena: true}, nil)
	if x.Domen != nil {
		t.Fatalf("домен остался: %+v", x.Domen)
	}
}

func TestPutPodpiski(t *testing.T) {
	put := "0123456789abcdef0123456789abcdef"
	for ssylka, zhdali := range map[string]string{
		"https://kot.example.net:2087/" + put:  put,
		"https://kot.example.net/" + put:       put,
		"https://kot.example.net/" + put + "/": "",
		"https://kot.example.net/abc":          "",
		"":                                     "",
		"::":                                   "",
	} {
		if got := konfig.PutPodpiski(ssylka); got != zhdali {
			t.Errorf("%q: %q", ssylka, got)
		}
	}
}

func TestSvoyVpsDomenOtkaz(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _, _ := postavitDlyaPodbora(t, s, kh)
	vyzvan := false
	s.vpsSmenitRezhim = func(context.Context, upravlenie.SmenaRezhima) (upravlenie.Itog, error) {
		vyzvan = true
		return upravlenie.Itog{}, errors.New("не ждали")
	}
	for imya, telo := range map[string]any{
		"адрес вместо домена": map[string]string{"id": id, "domen": "203.0.113.10"},
		"одно слово":          map[string]string{"id": id, "domen": "kot"},
		"нет сервера":         map[string]string{"id": "svoy-0000000000000000", "domen": domenTesta},
		"тело не разобрано":   "id",
	} {
		if kod := kodOtkaza(s.svoyVpsDomen(kadrVps(t, "svoyVpsDomen", telo))); kod != protokol.KodTeloNegodno {
			t.Errorf("%s: код %q", imya, kod)
		}
	}
	if vyzvan {
		t.Fatal("смена пошла при негодной команде")
	}

	for _, c := range []struct {
		imya  string
		err   error
		tekst string
	}{
		{"сертификат", &upravlenie.SertDomenaNeVyshel{Domen: domenTesta, Otkaz: "проверка 80 не дошла"}, "проверка 80 не дошла"},
		{"другой домен", &upravlenie.ErrDrugoyDomen{Domen: "pes.example.net"}, "pes.example.net"},
		{"старая программа", upravlenie.ErrStarayaProgrammaDomena, "обнови"},
		{"сайт не встал", &upravlenie.SaytNeVstal{Prichina: "порт занят"}, "(порт занят), и домен не подключён"},
		{"откат не прошёл", &upravlenie.OtkatNeVyshel{Err: &upravlenie.SaytNeVstal{Prichina: "порт занят"}, Otkat: errors.New("обрыв")},
			"и домен не подключён. Вернуть сервер к прежним ключам не вышло"},
	} {
		s.vpsSmenitRezhim = func(context.Context, upravlenie.SmenaRezhima) (upravlenie.Itog, error) {
			return upravlenie.Itog{}, c.err
		}
		if otv := s.svoyVpsDomen(kadrVps(t, "svoyVpsDomen", map[string]string{"id": id, "domen": domenTesta})); otv.Oshib != nil {
			t.Fatal(otv.Oshib)
		}
		h := dozhdatsyaVps(t, s)
		if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsDomen || !strings.Contains(h.Otkaz.Tekst, c.tekst) {
			t.Errorf("%s: %+v", c.imya, h.Otkaz)
		}
	}
	if zap := zapisPoId(t, s, id); zap.Domen != "" || zap.Maska != "dl.google.com" {
		t.Fatalf("отказ тронул запись: %+v", zap)
	}
}

func TestSvoyVpsPodpiska(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, sek, bylo := naDomene(t, s, kh, false)
	if err := s.pravitNabor(func(n *Nabor) error {
		n.SvoiServery[0].NeIzSeti = []string{"reality-tcp", "hy2"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var p upravlenie.SmenaPodpiski
	s.vpsObnovitPodpisku = func(_ context.Context, x upravlenie.SmenaPodpiski) (upravlenie.Itog, error) {
		p = x
		it := itogDomena(t, sek, kh, false)
		return it, nil
	}
	if otv := s.svoyVpsPodpiska(kadrVps(t, "svoyVpsPodpiska", map[string]string{"id": id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	// Новую ссылку человек просил сам: предупреждать о ней незачем.
	if h.Faza != fazaGotovo || h.Vid != vidPodpiska || h.Domen == nil || h.Domen.Zachem != zachemPut || !p.NovyyPut || p.Routing != nil ||
		len(h.Preduprezhdeniya) != 0 {
		t.Fatalf("новая ссылка: %+v, смена %+v", h, p)
	}
	zap := zapisPoId(t, s, id)
	if zap.Podpiska == bylo.Vhody.SsylkaPodpiski() || !strings.HasPrefix(zap.Podpiska, "https://"+domenTesta+"/") {
		t.Fatalf("ссылка не сменилась: %q", zap.Podpiska)
	}
	// Ключи те же: отметки о сети остались все.
	if !slices.Equal(slices.Sorted(slices.Values(zap.NeIzSeti)), []string{"hy2", "reality-tcp"}) {
		t.Fatalf("отметки о сети: %v", zap.NeIzSeti)
	}

	// Пакет принят, а сайт молчит: запись всё равно берёт новую ссылку, а
	// человек видит предупреждение.
	var novaya string
	s.vpsObnovitPodpisku = func(context.Context, upravlenie.SmenaPodpiski) (upravlenie.Itog, error) {
		it := itogDomena(t, sek, kh, false)
		novaya = it.Vhody.SsylkaPodpiski()
		it.Preduprezhdeniya = []upravlenie.Preduprezhdenie{{Vid: upravlenie.SaytNeOtvetil, Domen: domenTesta, Otkaz: "HTTPS не слушает"}}
		return it, nil
	}
	if otv := s.svoyVpsPodpiska(kadrVps(t, "svoyVpsPodpiska", map[string]string{"id": id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h = dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || len(h.Preduprezhdeniya) != 1 || h.Preduprezhdeniya[0].Kod != protokol.KodVpsSaytNeOtvechaet ||
		!strings.Contains(h.Preduprezhdeniya[0].Tekst, "HTTPS не слушает") {
		t.Fatalf("сайт молчит: %+v", h.Preduprezhdeniya)
	}
	if zap := zapisPoId(t, s, id); zap.Podpiska != novaya {
		t.Fatalf("запись не взяла новую ссылку: %q", zap.Podpiska)
	}

	// У сервера с простыми ключами подписки нет.
	s2 := sPustymNaborom(t)
	id2, _, _ := postavitDlyaPodbora(t, s2, kh)
	if kod := kodOtkaza(s2.svoyVpsPodpiska(kadrVps(t, "svoyVpsPodpiska", map[string]string{"id": id2}))); kod != protokol.KodVpsDomen {
		t.Fatalf("простые ключи: %q", kod)
	}
}

// В режиме домена «российские сайты» идут заданием: запись меняется, когда
// подписка на сервере пересобрана, а не раньше.
func TestSvoyVpsObhodRfDomen(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, sek, _ := naDomene(t, s, kh, false)
	pusk := make(chan struct{})
	var p upravlenie.SmenaPodpiski
	s.vpsObnovitPodpisku = func(_ context.Context, x upravlenie.SmenaPodpiski) (upravlenie.Itog, error) {
		p = x
		<-pusk
		return itogDomena(t, sek, kh, true), nil
	}
	otv := s.svoyVpsObhodRf(kadrVps(t, "svoyVpsObhodRf", map[string]any{"id": id, "vkl": false}))
	if otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	var telo struct {
		Hod *HodVps `json:"hod"`
	}
	if err := json.Unmarshal(otv.Telo, &telo); err != nil || telo.Hod == nil || telo.Hod.Vid != vidPodpiska {
		t.Fatalf("ответ без задания: %s", otv.Telo)
	}
	if zapisPoId(t, s, id).RfCherezVpn {
		t.Fatal("запись сменилась раньше подписки")
	}
	close(pusk)
	h := dozhdatsyaVps(t, s)
	vykl, err := routingDlya(true)
	if err != nil {
		t.Fatal(err)
	}
	if h.Faza != fazaGotovo || h.Domen == nil || h.Domen.Zachem != zachemRf || h.Domen.Vkl == nil || *h.Domen.Vkl ||
		p.Routing == nil || *p.Routing != vykl || p.NovyyPut {
		t.Fatalf("задание %+v, смена %+v", h, p)
	}
	if !zapisPoId(t, s, id).RfCherezVpn {
		t.Fatal("выбор не записан после подписки")
	}

	// Подписка не пересобралась: запись прежняя.
	s.vpsObnovitPodpisku = func(context.Context, upravlenie.SmenaPodpiski) (upravlenie.Itog, error) {
		return upravlenie.Itog{}, errors.New("сервер не ответил")
	}
	if otv := s.svoyVpsObhodRf(kadrVps(t, "svoyVpsObhodRf", map[string]any{"id": id, "vkl": true})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	if h := dozhdatsyaVps(t, s); h.Faza != fazaOshibka || !zapisPoId(t, s, id).RfCherezVpn {
		t.Fatalf("отказ: %+v", h)
	}
}

// Переименование сервера на домене пересобирает подписку с новым названием.
func TestSvoyVpsPereimenovatDomen(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, sek, _ := naDomene(t, s, kh, false)
	var p upravlenie.SmenaPodpiski
	s.vpsObnovitPodpisku = func(_ context.Context, x upravlenie.SmenaPodpiski) (upravlenie.Itog, error) {
		p = x
		it := itogDomena(t, sek, kh, false)
		it.Vhody.Imya = x.Imya
		var err error
		if it.Klyuchi, err = konfig.Klyuchi(it.Sekrety, it.Vhody); err != nil {
			t.Fatal(err)
		}
		return it, nil
	}
	otv := s.svoyVpsPereimenovat(kadrVps(t, "svoyVpsPereimenovat", map[string]string{"id": id, "imya": "Офис"}))
	if otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	var telo struct {
		Hod             *HodVps   `json:"hod"`
		Preduprezhdenie *OtkazVps `json:"preduprezhdenie"`
	}
	if err := json.Unmarshal(otv.Telo, &telo); err != nil || telo.Hod == nil || telo.Preduprezhdenie != nil {
		t.Fatalf("ответ: %s", otv.Telo)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Domen == nil || h.Domen.Zachem != zachemImya || p.Imya != "Офис" || p.NovyyPut || p.Routing != nil {
		t.Fatalf("задание %+v, смена %+v", h, p)
	}
	if zap := zapisPoId(t, s, id); zap.Imya != "Офис" {
		t.Fatalf("название: %q", zap.Imya)
	}

	// Идёт проверка сервера: она записи целиком не пишет и переименование
	// пускает, а задание подписки уже не заведётся. Название сменилось,
	// подписка отстала, и это предупреждение, а не отказ.
	s.muVps.Lock()
	s.vps.gotovo, s.vps.snimok.Vid = false, vidProverka
	s.muVps.Unlock()
	otv = s.svoyVpsPereimenovat(kadrVps(t, "svoyVpsPereimenovat", map[string]string{"id": id, "imya": "Дом"}))
	s.muVps.Lock()
	s.vps.gotovo, s.vps.snimok.Vid = true, vidPodpiska
	s.muVps.Unlock()
	telo.Hod, telo.Preduprezhdenie = nil, nil
	if err := json.Unmarshal(otv.Telo, &telo); otv.Oshib != nil || err != nil || telo.Hod != nil ||
		telo.Preduprezhdenie == nil || telo.Preduprezhdenie.Kod != protokol.KodVpsZanyato {
		t.Fatalf("ответ: %s %v", otv.Telo, otv.Oshib)
	}
	if zap := zapisPoId(t, s, id); zap.Imya != "Дом" {
		t.Fatalf("название: %q", zap.Imya)
	}
}

// На сервере режима домена маска это сам домен, и смена маски отвергается
// без входа на сервер.
func TestSvoyVpsMaskaNaDomene(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _, _ := naDomene(t, s, kh, false)
	s.vpsSmenitMasku = func(context.Context, upravlenie.SmenaMaski) (upravlenie.Itog, error) {
		t.Fatal("смена маски пошла на сервер в режиме домена")
		return upravlenie.Itog{}, nil
	}
	if kod := kodOtkaza(s.svoyVpsMaska(kadrVps(t, "svoyVpsMaska", zaprosMaskiVps{Id: id}))); kod != protokol.KodVpsMaska {
		t.Fatalf("код %q", kod)
	}
}

func TestSvoyVpsDomenProverit(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, true)
	var sprosili []string
	s.vpsProveritDomen = func(_ context.Context, d string, a netip.Addr) domen.Itog {
		sprosili = append(sprosili, d+" "+a.String())
		return domen.Itog{Domen: d, Kod: domen.Sovpadaet, Adresa: []string{a.String()}}
	}
	proverit := func(telo any) protokol.Kadr {
		return s.svoyVpsDomenProverit(context.Background(), kadrVps(t, "svoyVpsDomenProverit", telo))
	}
	// В карточке адрес берётся из записи, в мастере - присланный.
	for _, telo := range []map[string]string{
		{"id": id, "domen": "Kot.Example.net", "adres": "198.51.100.7"},
		{"adres": " 203.0.113.10 ", "domen": "kot.example.net."},
	} {
		otv := proverit(telo)
		var it domen.Itog
		if err := json.Unmarshal(otv.Telo, &it); otv.Oshib != nil || err != nil || it.Kod != domen.Sovpadaet || it.Domen != domenTesta {
			t.Fatalf("%v: %+v %v", telo, it, otv.Oshib)
		}
	}
	if !slices.Equal(sprosili, []string{domenTesta + " " + adresSvoego, domenTesta + " " + adresSvoego}) {
		t.Fatalf("спросили %v", sprosili)
	}
	for imya, telo := range map[string]any{
		"не домен":    map[string]string{"adres": adresSvoego, "domen": "203.0.113.10"},
		"не адрес":    map[string]string{"adres": "kot.example.net", "domen": domenTesta},
		"нет сервера": map[string]string{"id": "svoy-0000000000000000", "domen": domenTesta},
		"не то тело":  []int{1},
	} {
		if kod := kodOtkaza(proverit(telo)); kod != protokol.KodTeloNegodno {
			t.Errorf("%s: код %q", imya, kod)
		}
	}
	if len(sprosili) != 2 {
		t.Fatalf("негодная команда спросила DNS: %v", sprosili)
	}
}

func TestRfIzRoutinga(t *testing.T) {
	vkl, err := routingDlya(false)
	if err != nil {
		t.Fatal(err)
	}
	vykl, err := routingDlya(true)
	if err != nil {
		t.Fatal(err)
	}
	if rf := rfIzRoutinga(vkl); rf == nil || *rf {
		t.Fatalf("профиль «напрямую»: %v", rf)
	}
	if rf := rfIzRoutinga(vykl); rf == nil || !*rf {
		t.Fatalf("профиль «через VPN»: %v", rf)
	}
	for _, r := range []string{"", "happ://routing/add/чужой"} {
		if rf := rfIzRoutinga(r); rf != nil {
			t.Fatalf("%q узнан как %v", r, *rf)
		}
	}
}
