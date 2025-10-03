package storage

import (
	"context"
	"fmt"
	"log"

	"L3_5/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Storage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Storage {
	return &Storage{pool: pool}
}

func (s *Storage) CreateEvent(ctx context.Context, event *models.Event) error {
	const op = "storage.CreateEvent"

	// Normalize date to UTC to avoid timezone shifts when storing/retrieving
	event.Date = event.Date.UTC()
	log.Printf("%s: Creating event - Name: %s, Date: %s, Total Seats: %d, Payment Time: %d min",
		op, event.Name, event.Date.Format("2006-01-02 15:04:05"), event.TotalSeats, event.PaymentTime)

	// Return created_at as well so the caller has the timestamp that DB set
	query := `INSERT INTO events (name, date, total_seats, payment_time) 
			  VALUES ($1, $2, $3, $4) RETURNING id, created_at`

	err := s.pool.QueryRow(ctx, query,
		event.Name,
		event.Date,
		event.TotalSeats,
		event.PaymentTime).Scan(&event.ID, &event.CreatedAt)

	if err != nil {
		log.Printf("%s: Failed to insert event: %v", op, err)
		return fmt.Errorf("%s: %v", op, err)
	}

	log.Printf("%s: Successfully created event with ID: %d", op, event.ID)
	return nil
}

func (s *Storage) GetEvent(ctx context.Context, id int) (*models.Event, error) {
	const op = "storage.GetEvent"

	log.Printf("%s: Retrieving event with ID: %d", op, id)

	query := `SELECT id, name, date, total_seats, payment_time, created_at 
              FROM events WHERE id = $1`

	var event models.Event
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&event.ID,
		&event.Name,
		&event.Date,
		&event.TotalSeats,
		&event.PaymentTime,
		&event.CreatedAt,
	)
	if err != nil {
		log.Printf("%s: Failed to retrieve event ID %d: %v", op, id, err)
		return nil, fmt.Errorf("%s: %v", op, err)
	}

	log.Printf("%s: Successfully retrieved event ID %d: %s", op, event.ID, event.Name)
	return &event, nil
}

func (s *Storage) BookSeats(ctx context.Context, booking *models.Booking) error {
	const op = "storage.BookSeats"

	log.Printf("%s: Starting seat booking - User: %s, Seats: %d, Event ID: %d",
		op, booking.UserName, booking.Seats, booking.EventID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		log.Printf("%s: Failed to begin transaction: %v", op, err)
		return fmt.Errorf("%s: %v", op, err)
	}
	defer tx.Rollback(ctx)

	var available int
	err = tx.QueryRow(ctx, `
        SELECT total_seats - COALESCE(SUM(seats), 0) 
        FROM events LEFT JOIN bookings 
        ON events.id = bookings.event_id 
        AND bookings.status = 'confirmed'
        WHERE events.id = $1
        GROUP BY events.id`, booking.EventID).Scan(&available)

	if err != nil {
		log.Printf("%s: Failed to check available seats for event %d: %v", op, booking.EventID, err)
		return fmt.Errorf("%s: %v", op, err)
	}

	log.Printf("%s: Available seats for event %d: %d, requested: %d",
		op, booking.EventID, available, booking.Seats)

	if available < booking.Seats {
		log.Printf("%s: Not enough seats - Available: %d, Requested: %d, User: %s, Event: %d",
			op, available, booking.Seats, booking.UserName, booking.EventID)
		return fmt.Errorf("%s: not enough seats", op)
	}

	// Return id, status and created_at so booking struct reflects DB defaults
	query := `INSERT INTO bookings (event_id, user_name, seats) 
			  VALUES ($1, $2, $3) RETURNING id, status, created_at`

	err = tx.QueryRow(ctx, query,
		booking.EventID,
		booking.UserName,
		booking.Seats).Scan(&booking.ID, &booking.Status, &booking.CreatedAt)

	if err != nil {
		log.Printf("%s: Failed to insert booking: %v", op, err)
		return fmt.Errorf("%s: %v", op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("%s: Failed to commit booking transaction: %v", op, err)
		return fmt.Errorf("%s: %v", op, err)
	}

	log.Printf("%s: Successfully created booking ID: %d for user: %s, seats: %d, event: %d",
		op, booking.ID, booking.UserName, booking.Seats, booking.EventID)
	return nil
}

func (s *Storage) ConfirmBooking(ctx context.Context, eventID int, userName string) error {
	const op = "storage.ConfirmBooking"

	log.Printf("%s: Confirming booking for user: %s, event ID: %d", op, userName, eventID)

	query := `UPDATE bookings SET status = 'confirmed' 
              WHERE event_id = $1 AND user_name = $2 AND status = 'pending'`

	res, err := s.pool.Exec(ctx, query, eventID, userName)
	if err != nil {
		log.Printf("%s: Failed to update booking status: %v", op, err)
		return fmt.Errorf("%s: %v", op, err)
	}

	rowsAffected := res.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("%s: No pending booking found for user: %s, event ID: %d", op, userName, eventID)
		return fmt.Errorf("%s: booking not found", op)
	}

	log.Printf("%s: Successfully confirmed booking for user: %s, event ID: %d", op, userName, eventID)
	return nil
}

func (s *Storage) GetEventBookings(ctx context.Context, eventID int) ([]models.Booking, error) {
	const op = "storage.GetEventBookings"

	log.Printf("%s: Retrieving bookings for event ID: %d", op, eventID)

	query := `SELECT id, event_id, user_name, seats, status, created_at 
              FROM bookings WHERE event_id = $1`

	rows, err := s.pool.Query(ctx, query, eventID)
	if err != nil {
		log.Printf("%s: Failed to query bookings for event %d: %v", op, eventID, err)
		return nil, fmt.Errorf("%s: %v", op, err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		var b models.Booking
		err := rows.Scan(&b.ID, &b.EventID, &b.UserName, &b.Seats, &b.Status, &b.CreatedAt)
		if err != nil {
			log.Printf("%s: Failed to scan booking row: %v", op, err)
			return nil, fmt.Errorf("%s: %v", op, err)
		}
		bookings = append(bookings, b)
	}

	log.Printf("%s: Retrieved %d bookings for event ID: %d", op, len(bookings), eventID)
	return bookings, nil
}

func (s *Storage) CancelExpiredBookings(ctx context.Context) error {
    const op = "storage.CancelExpiredBookings"

    log.Printf("%s: Starting expired bookings cleanup", op)

    // Более простой и надежный запрос
    query := `UPDATE bookings 
              SET status = 'cancelled'
              FROM events
              WHERE bookings.event_id = events.id
              AND bookings.status = 'pending'
              AND bookings.created_at < (NOW() - (events.payment_time * INTERVAL '1 minute'))`

    res, err := s.pool.Exec(ctx, query)
    if err != nil {
        log.Printf("%s: Failed to cancel expired bookings: %v", op, err)
        return fmt.Errorf("%s: %v", op, err)
    }

    cancelledCount := res.RowsAffected()
    log.Printf("%s: Cancelled %d expired bookings", op, cancelledCount)
    return nil
}
func (s *Storage) GetAvailableSeats(ctx context.Context, eventID int) (int, error) {
	const op = "storage.GetAvailableSeats"

	log.Printf("%s: Calculating available seats for event ID: %d", op, eventID)

	query := `
        SELECT e.total_seats - COALESCE(SUM(b.seats), 0) 
        FROM events e
        LEFT JOIN bookings b ON e.id = b.event_id AND b.status = 'confirmed'
        WHERE e.id = $1
        GROUP BY e.id, e.total_seats
    `

	var available int
	err := s.pool.QueryRow(ctx, query, eventID).Scan(&available)
	if err != nil {
		log.Printf("%s: Failed to calculate available seats for event %d: %v", op, eventID, err)
		return 0, fmt.Errorf("%s: %v", op, err)
	}

	log.Printf("%s: Event ID %d has %d available seats", op, eventID, available)
	return available, nil
}

func (s *Storage) GetAllEvents(ctx context.Context) ([]models.Event, error) {
	const op = "storage.GetAllEvents"

	log.Printf("%s: Retrieving all events", op)

	query := `SELECT id, name, date, total_seats, payment_time, created_at FROM events ORDER BY date ASC`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		log.Printf("%s: Failed to query all events: %v", op, err)
		return nil, fmt.Errorf("%s: %v", op, err)
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		err := rows.Scan(
			&event.ID,
			&event.Name,
			&event.Date,
			&event.TotalSeats,
			&event.PaymentTime,
			&event.CreatedAt,
		)
		if err != nil {
			log.Printf("%s: Failed to scan event row: %v", op, err)
			return nil, fmt.Errorf("%s: %v", op, err)
		}
		events = append(events, event)
	}

	log.Printf("%s: Retrieved %d events", op, len(events))
	return events, nil
}
