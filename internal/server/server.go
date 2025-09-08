package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"L3_5/internal/storage"
	"L3_5/models"

	"github.com/labstack/echo/v4"
)

type Server struct {
	storage *storage.Storage
	e       *echo.Echo
}

func New(storage *storage.Storage) *Server {
	s := &Server{
		storage: storage,
		e:       echo.New(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.e.POST("/events", s.createEvent)
	s.e.GET("/events", s.getEvents) // Добавляем новый endpoint
	s.e.POST("/events/:id/book", s.bookEvent)
	s.e.POST("/events/:id/confirm", s.confirmBooking)
	s.e.GET("/events/:id", s.getEvent)
	s.e.Static("/", "web")
}

func (s *Server) Start(port string) error {
	return s.e.Start(":" + port)
}

func (s *Server) createEvent(c echo.Context) error {
	const op = "server.createEvent"

	var event models.Event
	if err := c.Bind(&event); err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	ctx := context.Background()
	if err := s.storage.CreateEvent(ctx, &event); err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	return c.JSON(http.StatusCreated, event)
}

func (s *Server) getEvents(c echo.Context) error {
	const op = "server.getEvents"

	ctx := context.Background()

	// Получаем список мероприятий
	events, err := s.storage.GetAllEvents(ctx)
	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	// Для каждого мероприятия получаем количество свободных мест
	for i := range events {
		available, err := s.storage.GetAvailableSeats(ctx, events[i].ID)
		if err != nil {
			return fmt.Errorf("%s: %v", op, err)
		}
		events[i].TotalSeats = available // Переиспользуем поле для отображения свободных мест
	}

	return c.JSON(http.StatusOK, events)
}

func (s *Server) bookEvent(c echo.Context) error {
	const op = "server.bookEvent"

	eventID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event ID")
	}

	var booking models.Booking
	if err := c.Bind(&booking); err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}
	booking.EventID = eventID

	ctx := context.Background()
	if err := s.storage.BookSeats(ctx, &booking); err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	return c.JSON(http.StatusCreated, booking)
}

func (s *Server) confirmBooking(c echo.Context) error {
	const op = "server.confirmBooking"

	eventID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event ID")
	}

	var request struct {
		UserName string `json:"user_name"`
	}
	if err := c.Bind(&request); err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	ctx := context.Background()
	if err := s.storage.ConfirmBooking(ctx, eventID, request.UserName); err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "confirmed"})
}

func (s *Server) getEvent(c echo.Context) error {
	const op = "server.getEvent"

	eventID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event ID")
	}

	ctx := context.Background()
	event, err := s.storage.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	bookings, err := s.storage.GetEventBookings(ctx, eventID)
	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	response := struct {
		Event    *models.Event    `json:"event"`
		Bookings []models.Booking `json:"bookings"`
	}{
		Event:    event,
		Bookings: bookings,
	}

	return c.JSON(http.StatusOK, response)
}

func (s *Server) StartBackgroundWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.storage.CancelExpiredBookings(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}
