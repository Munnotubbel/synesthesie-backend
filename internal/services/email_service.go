package services

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/ioutil"
	"mime"
	"net/smtp"
	"path/filepath"
	"time"

	"github.com/synesthesie/backend/internal/config"
)

type EmailService struct {
	cfg       *config.Config
	templates map[string]*template.Template
}

func NewEmailService(cfg *config.Config) *EmailService {
	service := &EmailService{
		cfg:       cfg,
		templates: make(map[string]*template.Template),
	}

	// Load email templates
	service.loadTemplates()

	return service
}

// loadTemplates loads all email templates
func (s *EmailService) loadTemplates() {
	templateFiles := []string{
		"registration_confirmation.html",
		"ticket_confirmation.html",
		"event_reminder.html",
		"cancellation_confirmation.html",
		"password_reset.html",
	}

	for _, file := range templateFiles {
		path := filepath.Join("templates", file)
		tmpl, err := template.ParseFiles(path)
		if err != nil {
			fmt.Printf("Failed to load template %s: %v\n", file, err)
			continue
		}
		s.templates[file] = tmpl
	}
}

// SendRegistrationConfirmation sends a registration confirmation email
func (s *EmailService) SendRegistrationConfirmation(to, name, username, email string) error {
	data := map[string]interface{}{
		"Name":     name,
		"Username": username,
		"Email":    email,
		"LoginURL": s.cfg.FrontendURL + "/login",
	}

	subject := "Willkommen bei Synesthesie!"
	return s.sendEmail(to, subject, "registration_confirmation.html", data)
}

// SendPasswordResetLinkEmail sends a styled HTML reset link email
func (s *EmailService) SendPasswordResetLinkEmail(to, name, resetURL string) error {
	data := map[string]interface{}{
		"Name":     name,
		"ResetURL": resetURL,
	}
	return s.sendEmail(to, "Passwort zurücksetzen", "password_reset.html", data)
}

// SendTicketConfirmation sends a ticket purchase confirmation email
func (s *EmailService) SendTicketConfirmation(to string, ticketData map[string]interface{}) error {
	subject := "Ticketbestätigung - Synesthesie"

	// Prepare both: inline CID image and Data-URI fallback for non-MSO clients
	// Render HTML once we have possibly injected Data-URI
	tmpl, exists := s.templates["ticket_confirmation.html"]
	if !exists {
		return fmt.Errorf("template %s not found", "ticket_confirmation.html")
	}
	// Try to load image bytes
	imgPath := filepath.Join("pictures", "lageplan.png")
	imgData, err := ioutil.ReadFile(imgPath)
	if err == nil {
		// Provide Data-URI fallback to template
		ticketData["LageplanDataURI"] = fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(imgData))
	}

	var htmlBody bytes.Buffer
	if err := tmpl.Execute(&htmlBody, ticketData); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err != nil {
		// If the image file was not found/readable, send HTML with Data-URI not set (template still renders)
		return s.sendEmail(to, subject, "ticket_confirmation.html", ticketData)
	}

	imgB64 := base64.StdEncoding.EncodeToString(imgData)

	// Build multipart/related message with inline image
	from := fmt.Sprintf("%s <%s>", s.cfg.SMTPFromName, s.cfg.SMTPFrom)
	subjectEnc := mime.BEncoding.Encode("UTF-8", subject)
	boundary := fmt.Sprintf("rel-%d", time.Now().UnixNano())

	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subjectEnc))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/related; type=\"text/html\"; boundary=%q\r\n", boundary))
	msg.WriteString("\r\n")

	// HTML part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	msg.WriteString(htmlBody.String())
	msg.WriteString("\r\n")

	// Inline image part (CID)
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: image/png; name=\"lageplan.png\"\r\n")
	msg.WriteString("Content-Transfer-Encoding: base64\r\n")
	msg.WriteString("Content-ID: <lageplan>\r\n")
	msg.WriteString("Content-Disposition: inline; filename=\"lageplan.png\"\r\n")
	msg.WriteString("Content-Location: lageplan.png\r\n\r\n")
	for i := 0; i < len(imgB64); i += 76 {
		end := i + 76
		if end > len(imgB64) {
			end = len(imgB64)
		}
		msg.WriteString(imgB64[i:end])
		msg.WriteString("\r\n")
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return s.sendSMTP(to, msg.Bytes())
}

// SendEventReminder sends an event reminder email
func (s *EmailService) SendEventReminder(to string, reminderData map[string]interface{}) error {
	subject := "Erinnerung: Dein Event steht bevor!"
	return s.sendEmail(to, subject, "event_reminder.html", reminderData)
}

// SendCancellationConfirmation sends a cancellation confirmation email
func (s *EmailService) SendCancellationConfirmation(to string, cancellationData map[string]interface{}) error {
	subject := "Stornierungsbestätigung - Synesthesie"
	return s.sendEmail(to, subject, "cancellation_confirmation.html", cancellationData)
}

// SendEventCancelled notifies users that an event was cancelled (full refund issued)
func (s *EmailService) SendEventCancelled(to string, data map[string]interface{}) error {
	subject := "Event abgesagt – vollständige Rückerstattung"
	return s.sendEmail(to, subject, "cancellation_confirmation.html", data)
}

// SendEventAnnouncement sends a short announcement for newly created events
func (s *EmailService) SendEventAnnouncement(to string, data map[string]interface{}) error {
	subject := "Neues Event bei Synesthesie"
	return s.sendEmail(to, subject, "event_reminder.html", data)
}

// sendEmail sends an email using the specified template
func (s *EmailService) sendEmail(to, subject, templateName string, data interface{}) error {
	// Get template
	tmpl, exists := s.templates[templateName]
	if !exists {
		return fmt.Errorf("template %s not found", templateName)
	}

	// Execute template
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Prepare email
	from := fmt.Sprintf("%s <%s>", s.cfg.SMTPFromName, s.cfg.SMTPFrom)
	subjectEnc := mime.BEncoding.Encode("UTF-8", subject)

	// Build email message
	message := fmt.Sprintf("From: %s\r\n", from)
	message += fmt.Sprintf("To: %s\r\n", to)
	message += fmt.Sprintf("Subject: %s\r\n", subjectEnc)
	message += "MIME-Version: 1.0\r\n"
	message += "Content-Type: text/html; charset=\"UTF-8\"\r\n"
	message += "\r\n"
	message += body.String()

	// Send email
	return s.sendSMTP(to, []byte(message))
}

// sendSMTP sends an email via SMTP
func (s *EmailService) sendSMTP(to string, message []byte) error {
	// Setup authentication
	auth := smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	// For TLS connection (port 465)
	if s.cfg.SMTPPort == 465 {
		// Create TLS config
		tlsConfig := &tls.Config{
			ServerName: s.cfg.SMTPHost,
		}

		// Connect with TLS
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		defer conn.Close()

		// Create SMTP client
		client, err := smtp.NewClient(conn, s.cfg.SMTPHost)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		defer client.Close()

		// Authenticate
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}

		// Set sender and recipient
		if err := client.Mail(s.cfg.SMTPFrom); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}

		// Send message
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(message)
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			return err
		}

		return client.Quit()
	}

	// For STARTTLS connection (port 587)
	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{to}, message)
}

// SendGenericTextEmail sends a plain text email with given subject and body
func (s *EmailService) SendGenericTextEmail(to, subject, body string) error {
	from := fmt.Sprintf("%s <%s>", s.cfg.SMTPFromName, s.cfg.SMTPFrom)
	subjectEnc := mime.BEncoding.Encode("UTF-8", subject)
	message := fmt.Sprintf("From: %s\r\n", from)
	message += fmt.Sprintf("To: %s\r\n", to)
	message += fmt.Sprintf("Subject: %s\r\n", subjectEnc)
	message += "Content-Type: text/plain; charset=\"UTF-8\"\r\n"
	message += "\r\n"
	message += body
	return s.sendSMTP(to, []byte(message))
}

// SendPasswordResetEmail keeps backward-compat for admin-triggered resets (plain text)
func (s *EmailService) SendPasswordResetEmail(to, name, newPassword string) error {
	// Simple text email for password reset
	subject := "Passwort zurückgesetzt - Synesthesie"

	body := fmt.Sprintf(`Hallo %s,

Dein Passwort wurde erfolgreich zurückgesetzt.

Dein neues Passwort lautet: %s

Bitte melde dich mit diesem Passwort an und ändere es umgehend in deinen Kontoeinstellungen.

Mit freundlichen Grüßen,
Dein Synesthesie-Team`, name, newPassword)

	// Build email message
	from := fmt.Sprintf("%s <%s>", s.cfg.SMTPFromName, s.cfg.SMTPFrom)
	message := fmt.Sprintf("From: %s\r\n", from)
	message += fmt.Sprintf("To: %s\r\n", to)
	message += fmt.Sprintf("Subject: %s\r\n", subject)
	message += "Content-Type: text/plain; charset=\"UTF-8\"\r\n"
	message += "\r\n"
	message += body

	return s.sendSMTP(to, []byte(message))
}
