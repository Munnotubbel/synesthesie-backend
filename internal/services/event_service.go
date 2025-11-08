package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type EventService struct {
	db *gorm.DB
}

func NewEventService(db *gorm.DB) *EventService {
	return &EventService{db: db}
}

// GetDB returns the database instance
func (s *EventService) GetDB() *gorm.DB {
	return s.db
}

// helper: compose Date (Y-M-D) with HH:MM in Europe/Berlin
func (s *EventService) composeDateTime(date time.Time, hm string) (time.Time, error) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		loc = time.Local
	}
	date = date.In(loc)
	if len(hm) < 4 {
		return date, errors.New("invalid time format")
	}
	var hh, mm int
	// accept "HH:MM"
	if _, perr := fmt.Sscanf(hm, "%02d:%02d", &hh, &mm); perr != nil {
		return date, errors.New("invalid time format")
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hh, mm, 0, 0, loc), nil
}

// GetTurnoverByEventIDs returns a map[eventID]turnover (sum of total_amount for paid tickets)
func (s *EventService) GetTurnoverByEventIDs(eventIDs []uuid.UUID) (map[uuid.UUID]float64, error) {
	result := make(map[uuid.UUID]float64)
	if len(eventIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		EventID uuid.UUID
		Sum     float64
	}
	err := s.db.Model(&models.Ticket{}).
		Select("event_id, COALESCE(SUM(total_amount), 0) as sum").
		Where("status = ?", "paid").
		Where("event_id IN ?", eventIDs).
		Group("event_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.EventID] = r.Sum
	}
	return result, nil
}

// CreateEvent creates a new event
func (s *EventService) CreateEvent(event *models.Event) error {
	// Compose DateFrom/DateTo using TimeFrom/TimeTo
	df, err := s.composeDateTime(event.DateFrom, event.TimeFrom)
	if err != nil {
		return errors.New("invalid time_from format; expected HH:MM")
	}
	dt, err := s.composeDateTime(event.DateTo, event.TimeTo)
	if err != nil {
		return errors.New("invalid time_to format; expected HH:MM")
	}
	event.DateFrom = df
	event.DateTo = dt

	// Validate event dates
	if event.DateFrom.After(event.DateTo) {
		return errors.New("start date must be before end date")
	}

	if event.MaxParticipants <= 0 {
		return errors.New("max participants must be greater than 0")
	}

	if event.AllowedGroup == "" {
		event.AllowedGroup = "all"
	}
	if event.AllowedGroup != "all" && event.AllowedGroup != "guests" && event.AllowedGroup != "bubble" && event.AllowedGroup != "plus" {
		return errors.New("invalid allowed_group; must be 'all', 'guests', 'bubble' or 'plus'")
	}

	// Default prices if unset
	if event.GuestsPrice <= 0 {
		event.GuestsPrice = 100
	}
	if event.BubblePrice <= 0 {
		event.BubblePrice = 35
	}
	if event.PlusPrice <= 0 {
		event.PlusPrice = 50
	}
	if event.GuestsPrice < 0 || event.BubblePrice < 0 || event.PlusPrice < 0 {
		return errors.New("prices cannot be negative")
	}

	return s.db.Create(event).Error
}

// GetEventByID retrieves an event by ID
func (s *EventService) GetEventByID(eventID uuid.UUID) (*models.Event, error) {
	var event models.Event
	if err := s.db.First(&event, eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("event not found")
		}
		return nil, err
	}
	return &event, nil
}

// UpdateEvent updates an existing event
func (s *EventService) UpdateEvent(eventID uuid.UUID, updates map[string]interface{}) error {
	// Load current event
	var ev models.Event
	if err := s.db.First(&ev, eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("event not found")
		}
		return err
	}

	// Apply updates to struct
	if v, ok := updates["name"].(string); ok && v != "" {
		ev.Name = v
	}
	if v, ok := updates["description"].(string); ok {
		ev.Description = v
	}
	if v, ok := updates["date_from"].(time.Time); ok && !v.IsZero() {
		ev.DateFrom = v
	}
	if v, ok := updates["date_to"].(time.Time); ok && !v.IsZero() {
		ev.DateTo = v
	}
	if v, ok := updates["time_from"].(string); ok && v != "" {
		ev.TimeFrom = v
	}
	if v, ok := updates["time_to"].(string); ok && v != "" {
		ev.TimeTo = v
	}
	if v, ok := updates["max_participants"].(int); ok && v > 0 {
		ev.MaxParticipants = v
	}
	if v, ok := updates["allowed_group"].(string); ok && v != "" {
		ev.AllowedGroup = v
	}
	if v, ok := updates["guests_price"].(float64); ok {
		ev.GuestsPrice = v
	}
	if v, ok := updates["bubble_price"].(float64); ok {
		ev.BubblePrice = v
	}
	if v, ok := updates["plus_price"].(float64); ok {
		ev.PlusPrice = v
	}

	// Compose new DateFrom/DateTo using possibly updated times
	df, err := s.composeDateTime(ev.DateFrom, ev.TimeFrom)
	if err != nil {
		return errors.New("invalid time_from format; expected HH:MM")
	}
	dt, err := s.composeDateTime(ev.DateTo, ev.TimeTo)
	if err != nil {
		return errors.New("invalid time_to format; expected HH:MM")
	}
	ev.DateFrom = df
	ev.DateTo = dt

	// Validations
	if ev.DateFrom.After(ev.DateTo) {
		return errors.New("start date must be before end date")
	}
	if ev.MaxParticipants <= 0 {
		return errors.New("max participants must be greater than 0")
	}
	if ev.AllowedGroup != "all" && ev.AllowedGroup != "guests" && ev.AllowedGroup != "bubble" && ev.AllowedGroup != "plus" {
		return errors.New("invalid allowed_group; must be 'all', 'guests', 'bubble' or 'plus'")
	}
	if ev.GuestsPrice < 0 || ev.BubblePrice < 0 || ev.PlusPrice < 0 {
		return errors.New("prices cannot be negative")
	}

	return s.db.Model(&models.Event{}).Where("id = ?", eventID).Updates(map[string]interface{}{
		"name":             ev.Name,
		"description":      ev.Description,
		"date_from":        ev.DateFrom,
		"date_to":          ev.DateTo,
		"time_from":        ev.TimeFrom,
		"time_to":          ev.TimeTo,
		"max_participants": ev.MaxParticipants,
		"allowed_group":    ev.AllowedGroup,
		"guests_price":     ev.GuestsPrice,
		"bubble_price":     ev.BubblePrice,
		"plus_price":       ev.PlusPrice,
	}).Error
}

// DeleteEvent deletes an event
func (s *EventService) DeleteEvent(eventID uuid.UUID) error {
	// Check if event has any tickets
	var ticketCount int64
	if err := s.db.Model(&models.Ticket{}).Where("event_id = ?", eventID).Count(&ticketCount).Error; err != nil {
		return err
	}

	if ticketCount > 0 {
		return errors.New("cannot delete event with existing tickets")
	}

	result := s.db.Delete(&models.Event{}, eventID)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("event not found")
	}

	return nil
}

// DeactivateEvent deactivates an event
func (s *EventService) DeactivateEvent(eventID uuid.UUID) error {
	result := s.db.Model(&models.Event{}).Where("id = ?", eventID).Update("is_active", false)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("event not found")
	}

	return nil
}

// GetUpcomingEvents retrieves upcoming active events
func (s *EventService) GetUpcomingEvents(offset, limit int) ([]*models.Event, int64, error) {
	var events []*models.Event
	var total int64

	now := time.Now()
	query := s.db.Model(&models.Event{}).Where("is_active = ? AND date_from > ?", true, now)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order("date_from ASC").Find(&events).Error; err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// GetAllEvents retrieves all events with pagination
func (s *EventService) GetAllEvents(offset, limit int, includeInactive bool) ([]*models.Event, int64, error) {
	var events []*models.Event
	var total int64

	query := s.db.Model(&models.Event{})
	if !includeInactive {
		query = query.Where("is_active = ?", true)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order("date_from DESC").Find(&events).Error; err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// GetEventWithAvailability retrieves an event with availability information
func (s *EventService) GetEventWithAvailability(eventID uuid.UUID) (*models.Event, int, error) {
	event, err := s.GetEventByID(eventID)
	if err != nil {
		return nil, 0, err
	}

	availableSpots := event.GetAvailableSpots(s.db)
	return event, availableSpots, nil
}

// GetEventsByDateRange retrieves events within a date range
func (s *EventService) GetEventsByDateRange(startDate, endDate time.Time) ([]*models.Event, error) {
	var events []*models.Event

	err := s.db.Where("is_active = ? AND date_from >= ? AND date_to <= ?", true, startDate, endDate).
		Order("date_from ASC").Find(&events).Error

	return events, err
}

// CheckEventAvailability checks if an event has available spots
func (s *EventService) CheckEventAvailability(eventID uuid.UUID) (bool, int, error) {
	_, availableSpots, err := s.GetEventWithAvailability(eventID)
	if err != nil {
		return false, 0, err
	}

	return availableSpots > 0, availableSpots, nil
}
