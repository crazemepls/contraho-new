package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Package-level so tests can shorten the timeout and trust a test certificate.
var (
	smtpTimeout  = 2 * time.Minute
	tlsConfigFor = func(host string) *tls.Config {
		return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
)

type mailConfig struct {
	To       string // comma-separated addresses
	From     string
	Host     string // host:port
	User     string
	Password string
	TLSMode  string // auto | none | starttls | tls
	Subject  string
}

func (c mailConfig) enabled() bool { return c.To != "" }

// recipients parses --mail-to into bare addresses.
func (c mailConfig) recipients() ([]string, error) {
	var out []string
	for _, part := range strings.Split(c.To, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		addr, err := mail.ParseAddress(part)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient %q: %w", part, err)
		}
		out = append(out, addr.Address)
	}
	if len(out) == 0 {
		return nil, errors.New("--mail-to has no valid address")
	}
	return out, nil
}

// validate runs at startup so a bad mail setup fails before the long run.
func (c mailConfig) validate(fileOutput string) error {
	if !c.enabled() {
		return nil
	}
	if fileOutput == "" {
		return errors.New("--mail-to needs --filename (the CSV is what gets attached)")
	}
	if c.From == "" {
		return errors.New("--mail-to needs --mail-from")
	}
	if _, err := mail.ParseAddress(c.From); err != nil {
		return fmt.Errorf("invalid --mail-from: %w", err)
	}
	if c.Host == "" {
		return errors.New("--mail-to needs --smtp-host (host:port)")
	}
	if _, _, err := net.SplitHostPort(c.Host); err != nil {
		return fmt.Errorf("--smtp-host must look like host:port: %w", err)
	}
	switch c.TLSMode {
	case "auto", "none", "starttls", "tls":
	default:
		return fmt.Errorf("--smtp-tls must be auto, none, starttls or tls (got %q)", c.TLSMode)
	}
	if c.User != "" && c.Password == "" {
		return errors.New("--smtp-user is set but $SMTP_PASSWORD is empty")
	}
	_, err := c.recipients()
	return err
}

// buildMessage creates a multipart email: a short text body plus the CSV attachment.
func buildMessage(c mailConfig, to []string, attachmentPath, body string) ([]byte, error) {
	data, err := os.ReadFile(attachmentPath)
	if err != nil {
		return nil, fmt.Errorf("read attachment: %w", err)
	}
	from, err := mail.ParseAddress(c.From)
	if err != nil {
		return nil, fmt.Errorf("invalid --mail-from: %w", err)
	}
	subject := c.Subject
	if subject == "" {
		subject = "SonarQube project report " + time.Now().Format("2006-01-02")
	}
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)

	var parts bytes.Buffer
	mw := multipart.NewWriter(&parts)

	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", `text/plain; charset="utf-8"`)
	textHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	textPart, err := mw.CreatePart(textHeader)
	if err != nil {
		return nil, err
	}
	qp := quotedprintable.NewWriter(textPart)
	if _, err := qp.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}

	name := filepath.Base(attachmentPath)
	contentType := mime.FormatMediaType("text/csv", map[string]string{"charset": "utf-8", "name": name})
	if contentType == "" {
		contentType = "text/csv"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disposition == "" {
		disposition = "attachment"
	}
	attHeader := textproto.MIMEHeader{}
	attHeader.Set("Content-Type", contentType)
	attHeader.Set("Content-Transfer-Encoding", "base64")
	attHeader.Set("Content-Disposition", disposition)
	attPart, err := mw.CreatePart(attHeader)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 { // base64 lines must be at most 76 characters
		if _, err := attPart.Write([]byte(encoded[:76] + "\r\n")); err != nil {
			return nil, err
		}
		encoded = encoded[76:]
	}
	if _, err := attPart.Write([]byte(encoded + "\r\n")); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	domain := "localhost"
	if i := strings.LastIndex(from.Address, "@"); i >= 0 {
		domain = from.Address[i+1:]
	}
	toHeader := make([]string, len(to))
	for i, address := range to {
		toHeader[i] = (&mail.Address{Address: address}).String()
	}

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from.String())
	fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(toHeader, ", "))
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Message-ID: <%d.%d@%s>\r\n", time.Now().UnixNano(), os.Getpid(), domain)
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=%s\r\n\r\n", mw.Boundary())
	msg.Write(parts.Bytes())
	return msg.Bytes(), nil
}

// sendMail sends the attachment (and body) using the SMTP settings in c.
func sendMail(c mailConfig, attachmentPath, body string) error {
	to, err := c.recipients()
	if err != nil {
		return err
	}
	msg, err := buildMessage(c, to, attachmentPath, body)
	if err != nil {
		return err
	}
	from, _ := mail.ParseAddress(c.From)

	host, port, err := net.SplitHostPort(c.Host)
	if err != nil {
		return fmt.Errorf("invalid smtp host: %w", err)
	}
	mode := c.TLSMode
	if mode == "auto" {
		if port == "465" {
			mode = "tls"
		} else {
			mode = "opportunistic" // STARTTLS if the server offers it, otherwise plain
		}
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if mode == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", c.Host, tlsConfigFor(host))
	} else {
		conn, err = dialer.Dial("tcp", c.Host)
	}
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.Host, err)
	}
	defer conn.Close()
	// One overall deadline: a dead server must not hang the run forever.
	if err := conn.SetDeadline(time.Now().Add(smtpTimeout)); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp greeting from %s: %w", c.Host, err)
	}
	defer client.Close()

	if mode != "tls" && mode != "none" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfigFor(host)); err != nil {
				return fmt.Errorf("STARTTLS: %w", err)
			}
		} else if mode == "starttls" {
			return errors.New("server does not offer STARTTLS (use --smtp-tls=none if that is intended)")
		}
	}

	if c.User != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("--smtp-user was given but the server does not offer AUTH")
		}
		if err := client.Auth(smtp.PlainAuth("", c.User, c.Password, host)); err != nil {
			return fmt.Errorf("authentication: %w", err)
		}
	}

	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, address := range to {
		if err := client.Rcpt(address); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", address, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("server rejected the message: %w", err)
	}
	return client.Quit()
}
