package storage

import (
	"context"
	"fmt"

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

	query := `INSERT INTO events (name, date, total_seats, payment_time) 
              VALUES ($1, $2, $3, $4) RETURNING id`

	err := s.pool.QueryRow(ctx, query,
		event.Name,
		event.Date,
		event.TotalSeats,
		event.PaymentTime).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}
	return nil
}

func (s *Storage) GetEvent(ctx context.Context, id int) (*models.Event, error) {
	const op = "storage.GetEvent"

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
		return nil, fmt.Errorf("%s: %v", op, err)
	}
	return &event, nil
}

func (s *Storage) BookSeats(ctx context.Context, booking *models.Booking) error {
	const op = "storage.BookSeats"

	tx, err := s.pool.Begin(ctx)
	if err != nil {
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
		return fmt.Errorf("%s: %v", op, err)
	}

	if available < booking.Seats {
		return fmt.Errorf("%s: not enough seats", op)
	}

	query := `INSERT INTO bookings (event_id, user_name, seats) 
              VALUES ($1, $2, $3) RETURNING id`

	err = tx.QueryRow(ctx, query,
		booking.EventID,
		booking.UserName,
		booking.Seats).Scan(&booking.ID)

	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	return tx.Commit(ctx)
}

func (s *Storage) ConfirmBooking(ctx context.Context, eventID int, userName string) error {
	const op = "storage.ConfirmBooking"

	query := `UPDATE bookings SET status = 'confirmed' 
              WHERE event_id = $1 AND user_name = $2 AND status = 'pending'`

	res, err := s.pool.Exec(ctx, query, eventID, userName)
	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("%s: booking not found", op)
	}
	return nil
}

func (s *Storage) GetEventBookings(ctx context.Context, eventID int) ([]models.Booking, error) {
	const op = "storage.GetEventBookings"

	query := `SELECT id, event_id, user_name, seats, status, created_at 
              FROM bookings WHERE event_id = $1`

	rows, err := s.pool.Query(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", op, err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		var b models.Booking
		err := rows.Scan(&b.ID, &b.EventID, &b.UserName, &b.Seats, &b.Status, &b.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", op, err)
		}
		bookings = append(bookings, b)
	}
	return bookings, nil
}

func (s *Storage) CancelExpiredBookings(ctx context.Context) error {
	const op = "storage.CancelExpiredBookings"

	query := `UPDATE bookings 
              SET status = 'cancelled'
              FROM events
              WHERE bookings.event_id = events.id
              AND bookings.status = 'pending'
              AND bookings.created_at < NOW() - INTERVAL '1 minute' * events.payment_time`

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}
	return nil
}

func (s *Storage) GetAvailableSeats(ctx context.Context, eventID int) (int, error) {
	const op = "storage.GetAvailableSeats"

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
		return 0, fmt.Errorf("%s: %v", op, err)
	}

	return available, nil
}

func (s *Storage) GetAllEvents(ctx context.Context) ([]models.Event, error) {
	const op = "storage.GetAllEvents"

	query := `SELECT id, name, date, total_seats, payment_time, created_at FROM events`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
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
			return nil, fmt.Errorf("%s: %v", op, err)
		}
		events = append(events, event)
	}

	return events, nil
}
