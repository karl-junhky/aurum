package vhody

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	stls "github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
)

// sdelatSert выпускает самоподписанный сертификат на имя или на IP.
func sdelatSert(t *testing.T, imya string) (tls.Certificate, string, string) {
	t.Helper()
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()),
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	if ip := net.ParseIP(imya); ip != nil {
		shablon.IPAddresses = []net.IP{ip}
	} else {
		shablon.DNSNames = []string{imya}
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		t.Fatal(err)
	}
	klyuchDer, err := x509.MarshalECPrivateKey(klyuch)
	if err != nil {
		t.Fatal(err)
	}
	cert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	klyuchPEM := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: klyuchDer}))
	para, err := tls.X509KeyPair([]byte(cert), []byte(klyuchPEM))
	if err != nil {
		t.Fatal(err)
	}
	return para, cert, klyuchPEM
}

// chtoOtdal делает рукопожатие и отдаёт сертификат, который предъявил
// сервер. С korni клиент проверяет его, как живой клиент по ссылке: imya
// это sni ссылки, адрес IP в нём имени в приветствии не шлёт. Без korni это
// сканер, которому всё равно, что ему предъявили.
func chtoOtdal(t *testing.T, server func(net.Conn) *tls.Conn, imya string, korni *x509.CertPool) []byte {
	t.Helper()
	k, s := net.Pipe()
	defer k.Close()
	defer s.Close()
	go func() {
		tc := server(s)
		_ = tc.HandshakeContext(context.Background())
	}()
	nastroyki := &tls.Config{ServerName: imya, RootCAs: korni}
	if korni == nil {
		nastroyki.InsecureSkipVerify = true //nolint:gosec // сканер с чужим именем, смотрим, что ему отдали
	}
	klient := tls.Client(k, nastroyki)
	if err := klient.Handshake(); err != nil {
		t.Fatalf("рукопожатие с именем %q: %v", imya, err)
	}
	return klient.ConnectionState().PeerCertificates[0].Raw
}

func TestVyborSertifikata(t *testing.T) {
	t.Cleanup(func() { PostavitSerty(nil) })
	PostavitSerty(nil)

	fayl, faylCert, faylKlyuch := sdelatSert(t, "198.51.100.9")
	imennoy, _, _ := sdelatSert(t, "imya.example.net")
	svoy, _, _ := sdelatSert(t, "198.51.100.9")
	novyy, _, _ := sdelatSert(t, "198.51.100.9")

	opcii := &option.InboundTLSOptions{Enabled: true, Certificate: []string{faylCert}, Key: []string{faylKlyuch}}
	c, err := stls.NewServer(context.Background(), log.NewNOPFactory().NewLogger("tls"), *opcii)
	if err != nil {
		t.Fatal(err)
	}
	if err := podklyuchitSerty(c, opcii); err != nil {
		t.Fatal(err)
	}
	// Так держит настройки QUIC-слушатель hy2: одну копию с подъёма.
	zapomnennye, err := c.STDConfig()
	if err != nil {
		t.Fatal(err)
	}
	server := func(conn net.Conn) *tls.Conn { return tls.Server(conn, zapomnennye) }

	odinakovo := func(sluchay string, poluchen []byte, zhdali tls.Certificate) {
		t.Helper()
		if !bytes.Equal(poluchen, zhdali.Certificate[0]) {
			t.Errorf("%s: предъявлен не тот сертификат", sluchay)
		}
	}
	korni := x509.NewCertPool()
	for _, c := range []tls.Certificate{fayl, imennoy, svoy, novyy} {
		korni.AddCert(c.Leaf)
	}
	const ip = "198.51.100.9"
	odinakovo("без пакета", chtoOtdal(t, server, ip, korni), fayl)

	PostavitSerty(&Serty{Imya: "imya.example.net", PoImeni: &imennoy})
	odinakovo("только имя", chtoOtdal(t, server, "imya.example.net", korni), imennoy)

	PostavitSerty(&Serty{Imya: "imya.example.net", PoImeni: &imennoy, Svoy: &svoy})
	odinakovo("старая ссылка с именем", chtoOtdal(t, server, "imya.example.net", korni), imennoy)
	odinakovo("ссылка на IP", chtoOtdal(t, server, ip, korni), svoy)
	odinakovo("сканер с чужим именем", chtoOtdal(t, server, "chuzhoy.example.net", nil), svoy)

	// Продление: новый свой сертификат виден без перезапуска входа.
	PostavitSerty(&Serty{Svoy: &novyy})
	odinakovo("продлённый", chtoOtdal(t, server, ip, korni), novyy)
	odinakovo("сканер с именем, которого больше нет", chtoOtdal(t, server, "imya.example.net", nil), novyy)
}

func TestVyborSertifikataNeTrogaetReality(t *testing.T) {
	opcii := &option.InboundTLSOptions{Enabled: true, Reality: &option.InboundRealityOptions{Enabled: true}}
	if err := podklyuchitSerty(nil, opcii); err != nil {
		t.Fatal(err)
	}
	if err := podklyuchitSerty(nil, nil); err != nil {
		t.Fatal(err)
	}
}
