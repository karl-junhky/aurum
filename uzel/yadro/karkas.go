package yadro

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"sort"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/podpiska"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/sertip"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/vhody"
)

// Paket это то, что центр присылает серверу целиком.
//
// Версия 1 это только ядро. Во второй рядом едут сертификат имени подписки
// (он же может быть нужен входам ядра), сама подписка, которую сервер
// раздаёт людям, и свой сертификат сервера на его IP для TLS-входов. Третья
// это свой VPN без центра: сертификат на IP программа выпускает и продлевает
// сама (SertIPSam), а SertIP, если есть, это копия, сохранённая клиентом с
// прошлой установки. Proba делает пакет пробным (подбор портов).
type Paket struct {
	Versiya   int               `json:"versiya"`
	Yadro     json.RawMessage   `json:"yadro"` // конфиг sing-box как есть
	Sert      *podpiska.Sert    `json:"sert,omitempty"`
	Podpiska  *podpiska.Nabor   `json:"podpiska,omitempty"`
	SertIP    *podpiska.Sert    `json:"sert_ip,omitempty"`
	SertIPSam *sertip.Nastroyka `json:"sert_ip_sam,omitempty"`
	Proba     *Proba            `json:"proba,omitempty"`

	tlsSert   *tls.Certificate // разобранный Sert
	tlsSertIP *tls.Certificate // разобранный SertIP
}

// Proba это пробный пакет подбора портов: ядро с портами-кандидатами живёт
// Srok секунд, порты его входов открыты временными правилами брандмауэра, на
// диск пакет не ложится. Не пришёл за срок окончательный пакет - сервер
// возвращается к последнему принятому, правила снимаются. Брошенный подбор
// не оставляет открытых портов.
type Proba struct {
	Srok int `json:"srok"` // секунды
}

// Пределы срока пробы: подбор проверяет кандидатов по одному с паузой, это
// минуты, а не часы.
const (
	ProbaMin = 10
	ProbaMax = 3600
)

const VersiyaPaketa = 3

func razobratPaket(syroe []byte) (Paket, error) {
	var p Paket
	dec := json.NewDecoder(bytes.NewReader(syroe))
	// Незнакомое поле значит, что центр новее программы. Молча выбросить его
	// значит применить не то, что прислано.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Paket{}, fmt.Errorf("пакет не разбирается: %w", err)
	}
	switch {
	case p.Versiya < 1 || p.Versiya > VersiyaPaketa:
		return Paket{}, fmt.Errorf("версия пакета %d, программа понимает с 1 по %d", p.Versiya, VersiyaPaketa)
	case p.Versiya == 1 && (p.Sert != nil || p.Podpiska != nil || p.SertIP != nil || p.SertIPSam != nil):
		return Paket{}, fmt.Errorf("в пакете версии 1 сертификат или подписка")
	case p.Versiya == 2 && p.SertIPSam != nil:
		return Paket{}, fmt.Errorf("в пакете версии 2 sert_ip_sam: самостоятельный выпуск есть с версии 3")
	case p.Versiya < 3 && p.Proba != nil:
		return Paket{}, fmt.Errorf("в пакете версии %d proba: пробный пакет есть с версии 3", p.Versiya)
	case p.Proba != nil && (p.Proba.Srok < ProbaMin || p.Proba.Srok > ProbaMax):
		return Paket{}, fmt.Errorf("срок пробы %d с, можно от %d до %d", p.Proba.Srok, ProbaMin, ProbaMax)
	}
	if p.SertIPSam != nil {
		if err := p.SertIPSam.Proverit(); err != nil {
			return Paket{}, err
		}
	}
	if len(bytes.TrimSpace(p.Yadro)) == 0 {
		return Paket{}, fmt.Errorf("в пакете нет конфига ядра")
	}
	sert, err := podpiska.Proverit(p.Podpiska, p.Sert)
	if err != nil {
		return Paket{}, err
	}
	p.tlsSert = sert
	if p.SertIP != nil {
		para, err := tls.X509KeyPair([]byte(p.SertIP.Cert), []byte(p.SertIP.Klyuch))
		if err != nil {
			return Paket{}, fmt.Errorf("свой сертификат негоден: %w", err)
		}
		if len(para.Leaf.IPAddresses) == 0 {
			return Paket{}, fmt.Errorf("свой сертификат не на IP")
		}
		if p.SertIPSam != nil && !slices.ContainsFunc(para.Leaf.IPAddresses, net.ParseIP(p.SertIPSam.IP).Equal) {
			return Paket{}, fmt.Errorf("копия своего сертификата не на %s из sert_ip_sam", p.SertIPSam.IP)
		}
		p.tlsSertIP = &para
	}
	return p, nil
}

// kopiyaSertIP это SertIP пакета для выпуска: в третьей версии он копия.
func (p Paket) kopiyaSertIP() *sertip.Pem {
	if p.SertIP == nil {
		return nil
	}
	return &sertip.Pem{Cert: p.SertIP.Cert, Klyuch: p.SertIP.Klyuch}
}

// serty это сертификаты пакета для TLS-входов.
func (p Paket) serty() *vhody.Serty {
	s := &vhody.Serty{PoImeni: p.tlsSert, Svoy: p.tlsSertIP}
	if p.tlsSert != nil && len(p.tlsSert.Leaf.DNSNames) > 0 {
		s.Imya = p.tlsSert.Leaf.DNSNames[0]
	}
	return s
}

// Karkas это конфиг ядра без людей наших входов, в одном написании.
//
// Если каркас прежний, всё, что поменялось, это люди, и их можно сменить на
// лету. Если нет, поменялся сам вход (порт, маска, ключи, маршруты), и нужен
// перезапуск ядра.
func Karkas(yadro []byte) (string, error) {
	var konfig map[string]any
	dec := json.NewDecoder(bytes.NewReader(yadro))
	// Числа как есть: через float64 большое число могло бы исказиться и
	// дать ложное «каркас прежний».
	dec.UseNumber()
	if err := dec.Decode(&konfig); err != nil {
		return "", fmt.Errorf("конфиг ядра не разбирается: %w", err)
	}
	if vhodySyrye, est := konfig["inbounds"].([]any); est {
		for _, syroy := range vhodySyrye {
			vhod, est := syroy.(map[string]any)
			if !est {
				return "", fmt.Errorf("вход в конфиге не объект")
			}
			if tip, _ := vhod["type"].(string); vhody.NashiTipy[tip] {
				delete(vhod, "users")
			}
		}
	}
	// encoding/json пишет ключи объектов по алфавиту, отсюда одно написание.
	itog, err := json.Marshal(konfig)
	if err != nil {
		return "", err
	}
	return string(itog), nil
}

// Vhod это вход ядра глазами здоровья сервера.
type Vhod struct {
	Teg        string `json:"teg"`
	Tip        string `json:"tip"`
	Port       int64  `json:"port,omitempty"`
	Lyudey     int    `json:"lyudey"`
	Soedineniy int    `json:"soedineniy"`
	// Maska это сайт, под который прячется вход reality; у других пусто.
	Maska string `json:"maska,omitempty"`
}

// opisatVhody достаёт из конфига то, что про входы стоит показать.
func opisatVhody(yadro []byte) []Vhod {
	var konfig struct {
		Inbounds []struct {
			Type       string            `json:"type"`
			Tag        string            `json:"tag"`
			ListenPort json.Number       `json:"listen_port"`
			Users      []json.RawMessage `json:"users"`
			Tls        struct {
				Reality struct {
					Enabled   bool `json:"enabled"`
					Handshake struct {
						Server string `json:"server"`
					} `json:"handshake"`
				} `json:"reality"`
			} `json:"tls"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(yadro, &konfig); err != nil {
		// Конфиг к этому месту уже принят ядром, так что сюда не попасть.
		// Если всё же попали, в здоровье просто не будет списка входов.
		return nil
	}
	itog := make([]Vhod, 0, len(konfig.Inbounds))
	for _, v := range konfig.Inbounds {
		port, _ := v.ListenPort.Int64()
		vh := Vhod{Teg: v.Tag, Tip: v.Type, Port: port, Lyudey: len(v.Users)}
		if v.Tls.Reality.Enabled {
			vh.Maska = v.Tls.Reality.Handshake.Server
		}
		itog = append(itog, vh)
	}
	sort.Slice(itog, func(i, j int) bool { return itog[i].Teg < itog[j].Teg })
	return itog
}
