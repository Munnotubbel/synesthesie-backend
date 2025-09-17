package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"path/filepath"

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
	return s.sendEmail(to, subject, "ticket_confirmation.html", ticketData)
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

	// Build email message
	message := fmt.Sprintf("From: %s\r\n", from)
	message += fmt.Sprintf("To: %s\r\n", to)
	message += fmt.Sprintf("Subject: %s\r\n", subject)
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
	message := fmt.Sprintf("From: %s\r\n", from)
	message += fmt.Sprintf("To: %s\r\n", to)
	message += fmt.Sprintf("Subject: %s\r\n", subject)
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
