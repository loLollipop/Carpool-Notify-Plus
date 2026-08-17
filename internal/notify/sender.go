package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Sender delivers a notification message to a channel.
type Sender interface {
	Send(ctx context.Context, title string, message string) error
}

// HTTPClient is the shared HTTP client with a 15s timeout.
var HTTPClient = &http.Client{Timeout: 15 * time.Second}

// GotifySender sends messages to a Gotify server.
type GotifySender struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// Send posts a message to Gotify.
func (sender GotifySender) Send(ctx context.Context, title string, message string) error {
	if sender.BaseURL == "" || sender.Token == "" {
		return fmt.Errorf("gotify is not configured")
	}
	client := sender.Client
	if client == nil {
		client = HTTPClient
	}

	endpoint := strings.TrimRight(sender.BaseURL, "/") + "/message?token=" + url.QueryEscape(sender.Token)
	payload := map[string]any{
		"title":    title,
		"message":  message,
		"priority": 5,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("gotify status %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

// IYUUSender sends messages via IYUU.
type IYUUSender struct {
	Token  string
	Client *http.Client
}

// Send posts a message to IYUU.
func (sender IYUUSender) Send(ctx context.Context, title string, message string) error {
	if sender.Token == "" {
		return fmt.Errorf("iyuu is not configured")
	}
	client := sender.Client
	if client == nil {
		client = HTTPClient
	}

	endpoint := "https://iyuu.cn/" + url.PathEscape(sender.Token) + ".send"
	payload := map[string]string{
		"text": title,
		"desp": message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("iyuu status %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(responseBody, &result); err == nil && result.ErrCode != 0 {
		return fmt.Errorf("iyuu errcode %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// SMTPSender sends email via SMTP (STARTTLS on typical submission ports).
type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

// Send delivers title/message to the configured operator recipients.
func (sender SMTPSender) Send(ctx context.Context, title string, message string) error {
	return sender.SendTo(ctx, sender.To, title, message)
}

// SendTo delivers title/message to explicit recipients.
func (sender SMTPSender) SendTo(ctx context.Context, recipients []string, title string, message string) error {
	if sender.Host == "" || sender.Port <= 0 || sender.From == "" {
		return fmt.Errorf("smtp is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fromAddress, err := mail.ParseAddress(strings.TrimSpace(sender.From))
	if err != nil || strings.TrimSpace(fromAddress.Address) == "" {
		return fmt.Errorf("smtp from address is invalid")
	}
	toAddresses := make([]string, 0, len(recipients))
	toHeaders := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}
		address, parseErr := mail.ParseAddress(recipient)
		if parseErr != nil || strings.TrimSpace(address.Address) == "" {
			return fmt.Errorf("smtp recipient address is invalid")
		}
		toAddresses = append(toAddresses, address.Address)
		toHeaders = append(toHeaders, address.String())
	}
	if len(toAddresses) == 0 {
		return fmt.Errorf("smtp recipients are empty")
	}

	addr := net.JoinHostPort(sender.Host, strconv.Itoa(sender.Port))
	auth := smtp.PlainAuth("", sender.Username, sender.Password, sender.Host)
	payload := buildSMTPMessage(fromAddress.String(), toHeaders, title, message)

	var sendErr error
	if sender.Port == 465 {
		sendErr = sendSMTPTLS(ctx, addr, sender.Host, auth, fromAddress.Address, toAddresses, payload)
	} else {
		sendErr = sendSMTPStartTLS(ctx, addr, sender.Host, auth, fromAddress.Address, toAddresses, payload)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return sendErr
}

func buildSMTPMessage(from string, to []string, subject string, body string) []byte {
	var builder strings.Builder
	builder.WriteString("From: " + sanitizeSMTPHeader(from) + "\r\n")
	builder.WriteString("To: " + sanitizeSMTPHeader(strings.Join(to, ", ")) + "\r\n")
	builder.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", sanitizeSMTPHeader(subject)) + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("\r\n")
	normalizedBody := strings.ReplaceAll(body, "\r\n", "\n")
	normalizedBody = strings.ReplaceAll(normalizedBody, "\r", "\n")
	builder.WriteString(strings.ReplaceAll(normalizedBody, "\n", "\r\n"))
	return []byte(builder.String())
}

func sanitizeSMTPHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func sendSMTPStartTLS(ctx context.Context, addr, host string, auth smtp.Auth, from string, to []string, message []byte) error {
	connection, stopContextWatch, err := dialSMTPConnection(ctx, addr)
	if err != nil {
		return err
	}
	defer stopContextWatch()
	defer connection.Close()

	client, err := smtp.NewClient(connection, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("smtp server does not support STARTTLS")
	}
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if err := client.StartTLS(tlsConfig); err != nil {
		return err
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp server does not support authentication")
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func sendSMTPTLS(ctx context.Context, addr, host string, auth smtp.Auth, from string, to []string, message []byte) error {
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	rawConnection, stopContextWatch, err := dialSMTPConnection(ctx, addr)
	if err != nil {
		return err
	}
	defer stopContextWatch()
	connection := tls.Client(rawConnection, tlsConfig)
	defer connection.Close()
	if err := connection.HandshakeContext(ctx); err != nil {
		return err
	}

	client, err := smtp.NewClient(connection, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func dialSMTPConnection(ctx context.Context, addr string) (net.Conn, func(), error) {
	dialer := net.Dialer{Timeout: 15 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, func() {}, err
	}
	deadline := time.Now().Add(30 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		_ = connection.Close()
		return nil, func() {}, err
	}
	stop := context.AfterFunc(ctx, func() {
		_ = connection.SetDeadline(time.Now())
	})
	return connection, func() { _ = stop() }, nil
}

// Registry maps channel names to senders.
type Registry struct {
	Gotify Sender
	IYUU   Sender
	SMTP   Sender
}

// Get returns a sender for the channel name.
func (registry Registry) Get(channel string) (Sender, bool) {
	switch channel {
	case "gotify":
		if registry.Gotify == nil {
			return nil, false
		}
		return registry.Gotify, true
	case "iyuu":
		if registry.IYUU == nil {
			return nil, false
		}
		return registry.IYUU, true
	case "smtp":
		if registry.SMTP == nil {
			return nil, false
		}
		return registry.SMTP, true
	default:
		return nil, false
	}
}

// ParseSMTPRecipients splits a comma-separated recipient list.
func ParseSMTPRecipients(raw string) []string {
	parts := strings.Split(raw, ",")
	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			recipients = append(recipients, part)
		}
	}
	return recipients
}
