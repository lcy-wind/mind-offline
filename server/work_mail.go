package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

type qqWorkMailer struct{ from, password string }

func configureWorkMailer() (workMailSender, error) {
	from, password := strings.TrimSpace(os.Getenv("MAIL_FROM")), strings.TrimSpace(os.Getenv("QQ_SMTP_AUTH_CODE"))
	if from == "" && password == "" {
		return nil, nil
	}
	if from != "coleliu888@qq.com" || password == "" {
		return nil, errors.New("invalid sender configuration")
	}
	return &qqWorkMailer{from: from, password: password}, nil
}
func mailFailure(err error, afterData bool) error {
	var reply *textproto.Error
	if errors.As(err, &reply) {
		return &workMailError{retryable: reply.Code >= 400 && reply.Code < 500}
	}
	return &workMailError{retryable: !afterData, uncertain: afterData}
}
func (m *qqWorkMailer) Send(ctx context.Context, t workTask) error {
	if !validReminderEmail(t.ReminderEmail) || t.ReminderAt == nil {
		return &workMailError{}
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: &tls.Config{ServerName: "smtp.qq.com", MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(ctx, "tcp", "smtp.qq.com:465")
	if err != nil {
		return mailFailure(err, false)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(20 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	conn.SetDeadline(deadline)
	client, err := smtp.NewClient(conn, "smtp.qq.com")
	if err != nil {
		return mailFailure(err, false)
	}
	defer client.Close()
	if err = client.Auth(smtp.PlainAuth("", m.from, m.password, "smtp.qq.com")); err != nil {
		return mailFailure(err, false)
	}
	if err = client.Mail(m.from); err != nil {
		return mailFailure(err, false)
	}
	if err = client.Rcpt(t.ReminderEmail); err != nil {
		return mailFailure(err, false)
	}
	writer, err := client.Data()
	if err != nil {
		return mailFailure(err, false)
	}
	if _, err = io.WriteString(writer, workMailMessage(m.from, t, time.Now())); err != nil {
		return mailFailure(err, false)
	}
	if err = writer.Close(); err != nil {
		return mailFailure(err, true)
	}
	// DATA accepted: a failed QUIT must not cause a duplicate email.
	_ = client.Quit()
	return nil
}
func workMailMessage(from string, t workTask, now time.Time) string {
	zone := time.FixedZone("北京时间", 8*3600)
	format := func(t *time.Time) string {
		if t == nil {
			return "暂未安排"
		}
		return t.In(zone).Format("2006-01-02 15:04") + "（北京时间）"
	}
	urgency := map[int]string{1: "不急", 2: "普通", 3: "紧急"}[t.Urgency]
	body := fmt.Sprintf("你设置的工作提醒到时间了。\n\n工作：%s\n计划时间：%s\n紧急程度：%s\n提醒时间：%s\n\n打开牛马痛苦区：https://mind-offline.duckdns.org/\n处理完记得标为完成，给自己少留一件事。\n", t.Title, format(t.PlannedAt), urgency, format(t.ReminderAt))
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var wrapped strings.Builder
	for len(encoded) > 76 {
		wrapped.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	wrapped.WriteString(encoded + "\r\n")
	subject := mime.QEncoding.Encode("UTF-8", "牛马痛苦区｜工作提醒："+strings.NewReplacer("\r", " ", "\n", " ").Replace(t.Title))
	reminderID := ""
	if t.ReminderAt != nil {
		reminderID = t.ReminderAt.UTC().Format(time.RFC3339Nano)
	}
	messageID := hash(t.ID + "|" + t.ReminderEmail + "|" + reminderID)
	return "From: " + (&mail.Address{Name: "牛马痛苦区", Address: from}).String() + "\r\nTo: " + (&mail.Address{Address: t.ReminderEmail}).String() + "\r\nSubject: " + subject + "\r\nDate: " + now.Format(time.RFC1123Z) + "\r\nMessage-ID: <" + messageID + "@mind-offline.duckdns.org>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" + wrapped.String()
}
