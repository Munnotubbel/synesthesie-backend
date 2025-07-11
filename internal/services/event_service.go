package services

import (
	"errors"
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

// CreateEvent creates a new event
func (s *EventService) CreateEvent(event *models.Event) error {
	// Validate event dates
	if event.DateFrom.After(event.DateTo) {
		return errors.New("start date must be before end date")
	}

	if event.MaxParticipants <= 0 {
		return errors.New("max participants must be greater than 0")
	}

	if event.Price < 0 {
		return errors.New("price cannot be negative")
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
	// Validate updates
	if dateFrom, ok := updates["date_from"].(time.Time); ok {
		if dateTo, ok := updates["date_to"].(time.Time); ok {
			if dateFrom.After(dateTo) {
				return errors.New("start date must be before end date")
			}
		}
	}

	if maxParticipants, ok := updates["max_participants"].(int); ok && maxParticipants <= 0 {
		return errors.New("max participants must be greater than 0")
	}

	if price, ok := updates["price"].(float64); ok && price < 0 {
		return errors.New("price cannot be negative")
	}

	result := s.db.Model(&models.Event{}).Where("id = ?", eventID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("event not found")
	}

	return nil
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
