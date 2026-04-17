package smtp

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

type SMTPClient struct {
	host     string
	port     int
	username string
	password string
	useSSL   bool
}

func NewSMTPClient(host string, port int, username, password string, useSSL bool) *SMTPClient {
	return &SMTPClient{
		host:     host,
		port:     port,
		username: username,
		password: password,
		useSSL:   useSSL,
	}
}

func (c *SMTPClient) SendEmail(from string, to []string, cc []string, bcc []string, subject, body, contentType string) error {
	auth := smtp.PlainAuth("", c.username, c.password, c.host)

	// Build recipients list
	recipients := append([]string{}, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)

	// Build message headers
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = strings.Join(to, ", ")
	if len(cc) > 0 {
		headers["Cc"] = strings.Join(cc, ", ")
	}
	headers["Subject"] = subject

	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}
	headers["Content-Type"] = contentType

	// Build message
	msg := ""
	for k, v := range headers {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + body

	addr := fmt.Sprintf("%s:%d", c.host, c.port)

	if c.useSSL {
		return c.sendSMTPWithTLS(addr, auth, from, recipients, []byte(msg))
	}

	return smtp.SendMail(addr, auth, from, recipients, []byte(msg))
}

func (c *SMTPClient) TestConnection() error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)

	if c.useSSL {
		// Test TLS connection
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: c.host,
		})
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, c.host)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		defer client.Quit()

		// Try to authenticate
		auth := smtp.PlainAuth("", c.username, c.password, c.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	} else {
		// Test plain connection
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, c.host)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		defer client.Quit()

		// STARTTLS if available
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: c.host}); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		}

		// Try to authenticate
		auth := smtp.PlainAuth("", c.username, c.password, c.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	return nil
}

func (c *SMTPClient) sendSMTPWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, _ := net.SplitHostPort(addr)

	tlsConfig := &tls.Config{
		ServerName: host,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Quit()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return nil
}
