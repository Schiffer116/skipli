package auth

import (
	"fmt"
	"net/smtp"
)

type Mailer struct {
	from     string
	password string
	host     string
	port     string
}

func NewMailer(from, password string) *Mailer {
	return &Mailer{from: from, password: password, host: "smtp.gmail.com", port: "587"}
}

func (m *Mailer) Send(to, subject, body string) error {
	msg := fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s\r\n", to, subject, body)
	auth := smtp.PlainAuth("", m.from, m.password, m.host)
	return smtp.SendMail(m.host+":"+m.port, auth, m.from, []string{to}, []byte(msg))
}
