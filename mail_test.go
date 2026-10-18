package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- fake SMTP server ----------

type fakeMessage struct {
	from, data string
	rcpts      []string
	authed     bool
	usedTLS    bool
}

type fakeSMTP struct {
	ln         net.Listener
	tlsCfg     *tls.Config
	implicit   bool // TLS from the first byte (port 465 style)
	startTLS   bool // advertise STARTTLS
	wantUser   string
	wantPass   string // when wantUser != "" the server advertises AUTH PLAIN
	hang       bool   // accept the connection and never speak
	mu         sync.Mutex
	messages   []fakeMessage
	authFailed bool
}

func startFake(t *testing.T, f *fakeSMTP) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.ln = ln
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handle(conn)
		}
	}()
	return ln.Addr().String()
}

func (f *fakeSMTP) handle(conn net.Conn) {
	defer func() { conn.Close() }()
	if f.hang {
		time.Sleep(5 * time.Second)
		return
	}
	usedTLS := false
	if f.implicit {
		conn = tls.Server(conn, f.tlsCfg)
		usedTLS = true
	}
	reader := bufio.NewReader(conn)
	say := func(s string) { io.WriteString(conn, s+"\r\n") }
	say("220 fake ESMTP")
	var cur fakeMessage
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimRight(line, "\r\n")
		up := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			caps := []string{"fake"}
			if f.startTLS && !usedTLS {
				caps = append(caps, "STARTTLS")
			}
			if f.wantUser != "" {
				caps = append(caps, "AUTH PLAIN")
			}
			caps = append(caps, "8BITMIME")
			for i, c := range caps {
				sep := "-"
				if i == len(caps)-1 {
					sep = " "
				}
				say("250" + sep + c)
			}
		case up == "STARTTLS":
			say("220 go ahead")
			conn = tls.Server(conn, f.tlsCfg)
			reader = bufio.NewReader(conn)
			usedTLS = true
		case strings.HasPrefix(up, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(cmd[len("AUTH PLAIN"):]))
			if string(raw) == "\x00"+f.wantUser+"\x00"+f.wantPass {
				cur.authed = true
				say("235 2.7.0 ok")
			} else {
				f.mu.Lock()
				f.authFailed = true
				f.mu.Unlock()
				say("535 5.7.8 bad credentials")
			}
		case strings.HasPrefix(up, "MAIL FROM:"):
			cur.from = between(cmd) // ignores parameters such as BODY=8BITMIME
			say("250 ok")
		case strings.HasPrefix(up, "RCPT TO:"):
			cur.rcpts = append(cur.rcpts, between(cmd))
			say("250 ok")
		case up == "DATA":
			say("354 go")
			var data strings.Builder
			for {
				l, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				if strings.HasPrefix(l, "..") { // undo dot-stuffing
					l = l[1:]
				}
				data.WriteString(l)
			}
			cur.data = data.String()
			cur.usedTLS = usedTLS
			f.mu.Lock()
			f.messages = append(f.messages, cur)
			f.mu.Unlock()
			say("250 queued")
		case up == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

// between returns the text inside <...> of an SMTP command.
func between(cmd string) string {
	i, j := strings.Index(cmd, "<"), strings.Index(cmd, ">")
	if i < 0 || j < i {
		return cmd
	}
	return cmd[i+1 : j]
}

// selfSigned makes a certificate valid for 127.0.0.1 plus a pool that trusts it.
func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake smtp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, pool
}

// trustPool makes sendMail trust the test certificate for the duration of a test.
func trustPool(t *testing.T, pool *x509.CertPool) {
	old := tlsConfigFor
	tlsConfigFor = func(host string) *tls.Config {
		return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, RootCAs: pool}
	}
	t.Cleanup(func() { tlsConfigFor = old })
}

func sampleCSV(t *testing.T) (path string, content []byte) {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("Key,Name\n")
	for i := 0; i < 400; i++ { // long enough to need many base64 lines
		b.WriteString("tsc:proj" + string(rune('a'+i%26)) + ",Prøject é " + strings.Repeat("x", 20) + "\n")
	}
	path = filepath.Join(t.TempDir(), "report.csv")
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, b.Bytes()
}

// parse the received message: returns headers, decoded text body, decoded attachment
func parseReceived(t *testing.T, raw string) (mail.Header, string, []byte) {
	t.Helper()
	m, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		t.Fatalf("not multipart: %q %v", mediaType, err)
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	var text string
	var attachment []byte
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		// Go's multipart reader already decodes quoted-printable (and drops that header).
		switch p.Header.Get("Content-Transfer-Encoding") {
		case "base64":
			b, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
			attachment = b
		default:
			b, _ := io.ReadAll(p)
			text = string(b)
		}
	}
	return m.Header, text, attachment
}

func baseCfg(host string) mailConfig {
	return mailConfig{To: "a@example.com, B Person <b@example.com>", From: "Reports <reports@example.com>",
		Host: host, TLSMode: "none", Subject: "Weekly report"}
}

// ---------- tests ----------

func TestSendPlainNoAuth(t *testing.T) {
	f := &fakeSMTP{}
	addr := startFake(t, f)
	path, csv := sampleCSV(t)
	body := "Line one\n.\nLine three with ünicode\n" // a lone "." line must survive dot-stuffing
	if err := sendMail(baseCfg(addr), path, body); err != nil {
		t.Fatalf("sendMail: %v", err)
	}
	if len(f.messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(f.messages))
	}
	msg := f.messages[0]
	if msg.from != "reports@example.com" {
		t.Errorf("envelope from = %q", msg.from)
	}
	if strings.Join(msg.rcpts, ",") != "a@example.com,b@example.com" {
		t.Errorf("envelope rcpts = %v", msg.rcpts)
	}
	h, text, att := parseReceived(t, msg.data)
	if h.Get("Subject") != "Weekly report" {
		t.Errorf("subject = %q", h.Get("Subject"))
	}
	if !bytes.Equal(att, csv) {
		t.Errorf("attachment differs from the CSV (got %d bytes, want %d)", len(att), len(csv))
	}
	if strings.ReplaceAll(text, "\r\n", "\n") != body {
		t.Errorf("body mismatch: %q", text)
	}
}

func TestSendSTARTTLSWithAuth(t *testing.T) {
	cfg, pool := selfSigned(t)
	trustPool(t, pool)
	f := &fakeSMTP{tlsCfg: cfg, startTLS: true, wantUser: "bot", wantPass: "s3cret"}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.TLSMode, c.User, c.Password = "starttls", "bot", "s3cret"
	if err := sendMail(c, path, "hi"); err != nil {
		t.Fatalf("sendMail: %v", err)
	}
	if len(f.messages) != 1 || !f.messages[0].usedTLS || !f.messages[0].authed {
		t.Fatalf("want 1 authed message over TLS, got %+v", f.messages)
	}
}

func TestSendImplicitTLS(t *testing.T) {
	cfg, pool := selfSigned(t)
	trustPool(t, pool)
	f := &fakeSMTP{tlsCfg: cfg, implicit: true}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.TLSMode = "tls"
	if err := sendMail(c, path, "hi"); err != nil {
		t.Fatalf("sendMail: %v", err)
	}
	if len(f.messages) != 1 || !f.messages[0].usedTLS {
		t.Fatalf("want 1 message over TLS, got %+v", f.messages)
	}
}

func TestAutoUsesSTARTTLSWhenOffered(t *testing.T) {
	cfg, pool := selfSigned(t)
	trustPool(t, pool)
	f := &fakeSMTP{tlsCfg: cfg, startTLS: true}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.TLSMode = "auto"
	if err := sendMail(c, path, "hi"); err != nil {
		t.Fatalf("sendMail: %v", err)
	}
	if len(f.messages) != 1 || !f.messages[0].usedTLS {
		t.Fatalf("auto should have upgraded to TLS: %+v", f.messages)
	}
}

func TestAutoFallsBackToPlainWhenNoSTARTTLS(t *testing.T) {
	f := &fakeSMTP{}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.TLSMode = "auto"
	if err := sendMail(c, path, "hi"); err != nil {
		t.Fatalf("sendMail: %v", err)
	}
	if len(f.messages) != 1 || f.messages[0].usedTLS {
		t.Fatalf("expected one plain message: %+v", f.messages)
	}
}

func TestSTARTTLSRequiredButMissing(t *testing.T) {
	f := &fakeSMTP{}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.TLSMode = "starttls"
	err := sendMail(c, path, "hi")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("want a STARTTLS error, got %v", err)
	}
	if len(f.messages) != 0 {
		t.Fatal("nothing should have been sent")
	}
}

func TestUntrustedCertificateIsRejected(t *testing.T) {
	cfg, _ := selfSigned(t) // NOT trusted: default system roots are used
	f := &fakeSMTP{tlsCfg: cfg, startTLS: true}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.TLSMode = "starttls"
	err := sendMail(c, path, "hi")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("an untrusted certificate must be rejected, got %v", err)
	}
	if len(f.messages) != 0 {
		t.Fatal("nothing should have been sent")
	}
}

func TestWrongPassword(t *testing.T) {
	f := &fakeSMTP{wantUser: "bot", wantPass: "right"}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.User, c.Password = "bot", "wrong"
	err := sendMail(c, path, "hi")
	if err == nil || !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("want authentication error, got %v", err)
	}
	if len(f.messages) != 0 {
		t.Fatal("nothing should have been sent")
	}
}

func TestDeadServerTimesOut(t *testing.T) {
	old := smtpTimeout
	smtpTimeout = 400 * time.Millisecond
	t.Cleanup(func() { smtpTimeout = old })
	f := &fakeSMTP{hang: true}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	start := time.Now()
	err := sendMail(baseCfg(addr), path, "hi")
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v: the deadline did not work", time.Since(start))
	}
}

func TestSubjectCannotInjectHeaders(t *testing.T) {
	f := &fakeSMTP{}
	addr := startFake(t, f)
	path, _ := sampleCSV(t)
	c := baseCfg(addr)
	c.Subject = "Hi\r\nBcc: evil@example.com"
	if err := sendMail(c, path, "hi"); err != nil {
		t.Fatalf("sendMail: %v", err)
	}
	h, _, _ := parseReceived(t, f.messages[0].data)
	if h.Get("Bcc") != "" {
		t.Fatal("header injection through the subject worked")
	}
}

func TestValidate(t *testing.T) {
	ok := mailConfig{To: "a@example.com", From: "r@example.com", Host: "smtp.example.com:587", TLSMode: "auto"}
	cases := []struct {
		name string
		mod  func(c *mailConfig)
		file string
		bad  bool
	}{
		{"disabled needs nothing", func(c *mailConfig) { *c = mailConfig{} }, "", false},
		{"valid", func(c *mailConfig) {}, "out.csv", false},
		{"needs a filename", func(c *mailConfig) {}, "", true},
		{"needs from", func(c *mailConfig) { c.From = "" }, "out.csv", true},
		{"needs host", func(c *mailConfig) { c.Host = "" }, "out.csv", true},
		{"host needs a port", func(c *mailConfig) { c.Host = "smtp.example.com" }, "out.csv", true},
		{"bad recipient", func(c *mailConfig) { c.To = "not-an-address" }, "out.csv", true},
		{"bad tls mode", func(c *mailConfig) { c.TLSMode = "maybe" }, "out.csv", true},
		{"user without password", func(c *mailConfig) { c.User = "bot" }, "out.csv", true},
		{"user with password", func(c *mailConfig) { c.User, c.Password = "bot", "x" }, "out.csv", false},
	}
	for _, tc := range cases {
		c := ok
		tc.mod(&c)
		err := c.validate(tc.file)
		if (err != nil) != tc.bad {
			t.Errorf("%s: err=%v, wantErr=%v", tc.name, err, tc.bad)
		}
	}
}
