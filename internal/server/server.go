package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"L3_5/internal/storage"
	"L3_5/models"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

	// Add middleware for logging
	s.e.Use(middleware.Logger())
	s.e.Use(middleware.Recover())
	s.e.Use(middleware.RequestID())

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.e.POST("/events", s.createEvent)
	s.e.GET("/events", s.getEvents)
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
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	log.Printf("[%s] %s: Starting event creation request from IP: %s", requestID, op, c.RealIP())

	var event models.Event
	if err := c.Bind(&event); err != nil {
		log.Printf("[%s] %s: Failed to bind request data: %v", requestID, op, err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request data")
	}

	log.Printf("[%s] %s: Creating event - Name: %s, Date: %s, Total Seats: %d, Payment Time: %d min",
		requestID, op, event.Name, event.Date.Format("2006-01-02 15:04:05"), event.TotalSeats, event.PaymentTime)

	ctx := context.Background()
	if err := s.storage.CreateEvent(ctx, &event); err != nil {
		log.Printf("[%s] %s: Failed to create event in storage: %v", requestID, op, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create event")
	}

	log.Printf("[%s] %s: Successfully created event with ID: %d", requestID, op, event.ID)
	return c.JSON(http.StatusCreated, event)
}

func (s *Server) getEvents(c echo.Context) error {
	const op = "server.getEvents"
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	log.Printf("[%s] %s: Getting all events request from IP: %s", requestID, op, c.RealIP())

	ctx := context.Background()

	// Get list of events
	events, err := s.storage.GetAllEvents(ctx)
	if err != nil {
		log.Printf("[%s] %s: Failed to get events from storage: %v", requestID, op, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get events")
	}

	log.Printf("[%s] %s: Retrieved %d events from storage", requestID, op, len(events))

	// For each event, get available seats count
	type EventWithAvailableSeats struct {
		models.Event
		AvailableSeats int `json:"available_seats"`
	}

	var eventsWithSeats []EventWithAvailableSeats
	for _, event := range events {
		available, err := s.storage.GetAvailableSeats(ctx, event.ID)
		if err != nil {
			log.Printf("[%s] %s: Failed to get available seats for event ID %d: %v", requestID, op, event.ID, err)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get available seats")
		}
		eventsWithSeats = append(eventsWithSeats, EventWithAvailableSeats{
			Event:          event,
			AvailableSeats: available,
		})
	}

	log.Printf("[%s] %s: Successfully returned %d events with seat availability", requestID, op, len(eventsWithSeats))
	return c.JSON(http.StatusOK, eventsWithSeats)
}

func (s *Server) bookEvent(c echo.Context) error {
	const op = "server.bookEvent"
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	eventID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Printf("[%s] %s: Invalid event ID parameter: %s from IP: %s", requestID, op, c.Param("id"), c.RealIP())
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event ID")
	}

	log.Printf("[%s] %s: Starting seat booking for event ID: %d from IP: %s", requestID, op, eventID, c.RealIP())

	var booking models.Booking
	if err := c.Bind(&booking); err != nil {
		log.Printf("[%s] %s: Failed to bind booking request data: %v", requestID, op, err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid booking data")
	}
	booking.EventID = eventID

	log.Printf("[%s] %s: Booking request - User: %s, Seats: %d, Event ID: %d",
		requestID, op, booking.UserName, booking.Seats, booking.EventID)

	ctx := context.Background()
	if err := s.storage.BookSeats(ctx, &booking); err != nil {
		log.Printf("[%s] %s: Failed to book seats for user %s: %v", requestID, op, booking.UserName, err)
		if err.Error() == "storage.BookSeats: not enough seats" {
			return echo.NewHTTPError(http.StatusConflict, "Not enough available seats")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to book seats")
	}

	log.Printf("[%s] %s: Successfully created booking ID: %d for user: %s, seats: %d, event: %d",
		requestID, op, booking.ID, booking.UserName, booking.Seats, booking.EventID)
	return c.JSON(http.StatusCreated, booking)
}

func (s *Server) confirmBooking(c echo.Context) error {
	const op = "server.confirmBooking"
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	eventID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Printf("[%s] %s: Invalid event ID parameter: %s from IP: %s", requestID, op, c.Param("id"), c.RealIP())
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event ID")
	}

	log.Printf("[%s] %s: Starting booking confirmation for event ID: %d from IP: %s", requestID, op, eventID, c.RealIP())

	var request struct {
		UserName string `json:"user_name"`
	}
	if err := c.Bind(&request); err != nil {
		log.Printf("[%s] %s: Failed to bind confirmation request data: %v", requestID, op, err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request data")
	}

	log.Printf("[%s] %s: Confirming booking for user: %s, event ID: %d", requestID, op, request.UserName, eventID)

	ctx := context.Background()
	if err := s.storage.ConfirmBooking(ctx, eventID, request.UserName); err != nil {
		log.Printf("[%s] %s: Failed to confirm booking for user %s, event %d: %v", requestID, op, request.UserName, eventID, err)
		if err.Error() == "storage.ConfirmBooking: booking not found" {
			return echo.NewHTTPError(http.StatusNotFound, "Booking not found or already confirmed")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to confirm booking")
	}

	log.Printf("[%s] %s: Successfully confirmed booking for user: %s, event ID: %d", requestID, op, request.UserName, eventID)
	return c.JSON(http.StatusOK, map[string]string{"status": "confirmed"})
}

func (s *Server) getEvent(c echo.Context) error {
	const op = "server.getEvent"
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	eventID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Printf("[%s] %s: Invalid event ID parameter: %s from IP: %s", requestID, op, c.Param("id"), c.RealIP())
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event ID")
	}

	log.Printf("[%s] %s: Getting event details for ID: %d from IP: %s", requestID, op, eventID, c.RealIP())

	ctx := context.Background()
	event, err := s.storage.GetEvent(ctx, eventID)
	if err != nil {
		log.Printf("[%s] %s: Failed to get event ID %d: %v", requestID, op, eventID, err)
		return echo.NewHTTPError(http.StatusNotFound, "Event not found")
	}

	bookings, err := s.storage.GetEventBookings(ctx, eventID)
	if err != nil {
		log.Printf("[%s] %s: Failed to get bookings for event ID %d: %v", requestID, op, eventID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get event bookings")
	}

	availableSeats, err := s.storage.GetAvailableSeats(ctx, eventID)
	if err != nil {
		log.Printf("[%s] %s: Failed to get available seats for event ID %d: %v", requestID, op, eventID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get available seats")
	}

	response := struct {
		Event          *models.Event    `json:"event"`
		Bookings       []models.Booking `json:"bookings"`
		AvailableSeats int              `json:"available_seats"`
	}{
		Event:          event,
		Bookings:       bookings,
		AvailableSeats: availableSeats,
	}

	log.Printf("[%s] %s: Successfully returned event details for ID: %d with %d bookings and %d available seats",
		requestID, op, eventID, len(bookings), availableSeats)
	return c.JSON(http.StatusOK, response)
}

func (s *Server) StartBackgroundWorker(ctx context.Context) {
	log.Printf("Starting background worker for expired booking cleanup")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Printf("Running expired bookings cleanup...")
			if err := s.storage.CancelExpiredBookings(ctx); err != nil {
				log.Printf("Error during expired bookings cleanup: %v", err)
			} else {
				log.Printf("Expired bookings cleanup completed successfully")
			}
		case <-ctx.Done():
			log.Printf("Background worker shutting down")
			return
		}
	}
}
