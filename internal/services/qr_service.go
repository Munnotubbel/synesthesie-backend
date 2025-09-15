package services

import (
	"bytes"
	"fmt"

	"github.com/jung-kurt/gofpdf"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
)

type QRService struct {
	cfg *config.Config
}

func NewQRService(cfg *config.Config) *QRService { return &QRService{cfg: cfg} }

// GenerateInviteQRPDF generates a simple A4 PDF with a QR code for the invite link
func (s *QRService) GenerateInviteQRPDF(invite *models.InviteCode) ([]byte, error) {
	inviteURL := fmt.Sprintf("%s/register?invite=%s", s.cfg.FrontendURL, invite.Code)

	// Create QR PNG in memory
	var qrBuf bytes.Buffer
	png, err := qrcode.Encode(inviteURL, qrcode.Medium, 512)
	if err != nil {
		return nil, err
	}
	qrBuf.Write(png)

	// Create PDF
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 18)
	pdf.Cell(0, 10, "Synesthesie Invite")
	pdf.Ln(12)
	pdf.SetFont("Arial", "", 12)
	pdf.MultiCell(0, 6, fmt.Sprintf("Code: %s\nURL: %s", invite.Code, inviteURL), "", "L", false)

	// Register image from reader
	opt := gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
	pdf.RegisterImageOptionsReader("qr", opt, bytes.NewReader(qrBuf.Bytes()))

	// Center QR on the page
	x := (210.0 - 100.0) / 2.0 // A4 width 210mm, QR size 100mm
	y := pdf.GetY() + 10
	pdf.ImageOptions("qr", x, y, 100, 100, false, opt, 0, "")

	// Output to buffer
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
