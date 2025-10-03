package storage

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"testing"
	"time"

	"L3_5/models"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDB struct {
	Container testcontainers.Container
	Pool      *pgxpool.Pool
	Storage   *Storage
}

func setupTestDB(t *testing.T) *TestDB {
	ctx := context.Background()

	// Create PostgreSQL container
	postgresContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:15-alpine",
			Env: map[string]string{
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpass",
			},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)

	// Get connection details
	host, err := postgresContainer.Host(ctx)
	require.NoError(t, err)

	port, err := postgresContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)

	connStr := fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port())

	// Create connection pool
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	// Run migrations
	migrationPath := "file://" + filepath.Join("..", "..", "migrations")
	m, err := migrate.New(migrationPath, connStr)
	require.NoError(t, err)

	err = m.Up()
	require.NoError(t, err)

	// Create storage instance
	storage := New(pool)

	return &TestDB{
		Container: postgresContainer,
		Pool:      pool,
		Storage:   storage,
	}
}

func (tdb *TestDB) Cleanup(t *testing.T) {
	ctx := context.Background()
	if tdb.Pool != nil {
		tdb.Pool.Close()
	}
	if tdb.Container != nil {
		require.NoError(t, tdb.Container.Terminate(ctx))
	}
}

func TestCreateEvent(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	event := &models.Event{
		Name:        "Test Concert",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  100,
		PaymentTime: 30,
	}

	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)
	assert.NotZero(t, event.ID)
	assert.NotZero(t, event.CreatedAt)
}

func TestGetEvent(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event
	event := &models.Event{
		Name:        "Test Workshop",
		Date:        time.Now().Add(48 * time.Hour),
		TotalSeats:  50,
		PaymentTime: 15,
	}

	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Get the event
	retrievedEvent, err := tdb.Storage.GetEvent(ctx, event.ID)
	require.NoError(t, err)

	assert.Equal(t, event.ID, retrievedEvent.ID)
	assert.Equal(t, event.Name, retrievedEvent.Name)
	assert.Equal(t, event.TotalSeats, retrievedEvent.TotalSeats)
	assert.Equal(t, event.PaymentTime, retrievedEvent.PaymentTime)
	assert.WithinDuration(t, event.Date, retrievedEvent.Date, time.Second)
}

func TestGetEvent_NotFound(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	_, err := tdb.Storage.GetEvent(ctx, 999)
	require.Error(t, err)
}

func TestBookSeats_Success(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event
	event := &models.Event{
		Name:        "Test Event",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  100,
		PaymentTime: 30,
	}
	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Book seats
	booking := &models.Booking{
		EventID:  event.ID,
		UserName: "john_doe",
		Seats:    5,
	}

	err = tdb.Storage.BookSeats(ctx, booking)
	require.NoError(t, err)
	assert.NotZero(t, booking.ID)
	assert.Equal(t, "pending", booking.Status)
}

func TestBookSeats_NotEnoughSeats(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event with limited seats
	event := &models.Event{
		Name:        "Small Event",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  10,
		PaymentTime: 30,
	}
	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Book all seats
	booking1 := &models.Booking{
		EventID:  event.ID,
		UserName: "user1",
		Seats:    10,
	}
	err = tdb.Storage.BookSeats(ctx, booking1)
	require.NoError(t, err)

	// Confirm the booking to make seats unavailable
	err = tdb.Storage.ConfirmBooking(ctx, event.ID, "user1")
	require.NoError(t, err)

	// Try to book more seats than available
	booking2 := &models.Booking{
		EventID:  event.ID,
		UserName: "user2",
		Seats:    1,
	}
	err = tdb.Storage.BookSeats(ctx, booking2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enough seats")
}

func TestConfirmBooking_Success(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event
	event := &models.Event{
		Name:        "Test Event",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  100,
		PaymentTime: 30,
	}
	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Book seats
	booking := &models.Booking{
		EventID:  event.ID,
		UserName: "john_doe",
		Seats:    5,
	}
	err = tdb.Storage.BookSeats(ctx, booking)
	require.NoError(t, err)

	// Confirm booking
	err = tdb.Storage.ConfirmBooking(ctx, event.ID, "john_doe")
	require.NoError(t, err)

	// Verify booking is confirmed
	bookings, err := tdb.Storage.GetEventBookings(ctx, event.ID)
	require.NoError(t, err)
	require.Len(t, bookings, 1)
	assert.Equal(t, "confirmed", bookings[0].Status)
}

func TestConfirmBooking_NotFound(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event
	event := &models.Event{
		Name:        "Test Event",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  100,
		PaymentTime: 30,
	}
	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Try to confirm non-existent booking
	err = tdb.Storage.ConfirmBooking(ctx, event.ID, "nonexistent_user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "booking not found")
}

func TestGetEventBookings(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event
	event := &models.Event{
		Name:        "Test Event",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  100,
		PaymentTime: 30,
	}
	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Create multiple bookings
	bookings := []*models.Booking{
		{EventID: event.ID, UserName: "user1", Seats: 3},
		{EventID: event.ID, UserName: "user2", Seats: 5},
		{EventID: event.ID, UserName: "user3", Seats: 2},
	}

	for _, booking := range bookings {
		err = tdb.Storage.BookSeats(ctx, booking)
		require.NoError(t, err)
	}

	// Confirm one booking
	err = tdb.Storage.ConfirmBooking(ctx, event.ID, "user1")
	require.NoError(t, err)

	// Get all bookings
	retrievedBookings, err := tdb.Storage.GetEventBookings(ctx, event.ID)
	require.NoError(t, err)
	require.Len(t, retrievedBookings, 3)

	// Check statuses
	statusCount := make(map[string]int)
	for _, b := range retrievedBookings {
		statusCount[b.Status]++
	}
	assert.Equal(t, 1, statusCount["confirmed"])
	assert.Equal(t, 2, statusCount["pending"])
}

func TestCancelExpiredBookings(t *testing.T) {
    tdb := setupTestDB(t)
    defer tdb.Cleanup(t)

    ctx := context.Background()

    // Create test event with very short payment time (1 minute)
    event := &models.Event{
        Name:        "Test Event",
        Date:        time.Now().Add(24 * time.Hour),
        TotalSeats:  100,
        PaymentTime: 1, // 1 minute
    }
    err := tdb.Storage.CreateEvent(ctx, event)
    require.NoError(t, err)

    // Create booking
    booking := &models.Booking{
        EventID:  event.ID,
        UserName: "test_user",
        Seats:    5,
    }
    err = tdb.Storage.BookSeats(ctx, booking)
    require.NoError(t, err)

    // Manually set created_at to past to simulate expired booking
    // Используем время в UTC для согласованности
    expiredTime := time.Now().UTC().Add(-2 * time.Minute)
    _, err = tdb.Pool.Exec(ctx,
        "UPDATE bookings SET created_at = $1 WHERE id = $2",
        expiredTime, booking.ID)
    require.NoError(t, err)

    // Verify the booking was updated correctly
    var dbCreatedAt time.Time
    err = tdb.Pool.QueryRow(ctx, 
        "SELECT created_at FROM bookings WHERE id = $1", 
        booking.ID).Scan(&dbCreatedAt)
    require.NoError(t, err)
    
    log.Printf("Booking created_at set to: %v", dbCreatedAt)
    log.Printf("Current time (UTC): %v", time.Now().UTC())

    // Cancel expired bookings
    err = tdb.Storage.CancelExpiredBookings(ctx)
    require.NoError(t, err)

    // Verify booking is cancelled
    bookings, err := tdb.Storage.GetEventBookings(ctx, event.ID)
    require.NoError(t, err)
    require.Len(t, bookings, 1)
    assert.Equal(t, "cancelled", bookings[0].Status)
}

func TestCancelExpiredBookings_ConfirmedNotCancelled(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event with short payment time
	event := &models.Event{
		Name:        "Test Event",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  100,
		PaymentTime: 1, // 1 minute
	}
	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Create and confirm booking
	booking := &models.Booking{
		EventID:  event.ID,
		UserName: "test_user",
		Seats:    5,
	}
	err = tdb.Storage.BookSeats(ctx, booking)
	require.NoError(t, err)

	err = tdb.Storage.ConfirmBooking(ctx, event.ID, "test_user")
	require.NoError(t, err)

	// Manually set created_at to past
	_, err = tdb.Pool.Exec(ctx,
		"UPDATE bookings SET created_at = $1 WHERE id = $2",
		time.Now().Add(-2*time.Minute), booking.ID)
	require.NoError(t, err)

	// Cancel expired bookings
	err = tdb.Storage.CancelExpiredBookings(ctx)
	require.NoError(t, err)

	// Verify confirmed booking is NOT cancelled
	bookings, err := tdb.Storage.GetEventBookings(ctx, event.ID)
	require.NoError(t, err)
	require.Len(t, bookings, 1)
	assert.Equal(t, "confirmed", bookings[0].Status)
}

func TestGetAvailableSeats(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create test event
	event := &models.Event{
		Name:        "Test Event",
		Date:        time.Now().Add(24 * time.Hour),
		TotalSeats:  100,
		PaymentTime: 30,
	}
	err := tdb.Storage.CreateEvent(ctx, event)
	require.NoError(t, err)

	// Initially all seats should be available
	available, err := tdb.Storage.GetAvailableSeats(ctx, event.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, available)

	// Book and confirm some seats
	booking1 := &models.Booking{
		EventID:  event.ID,
		UserName: "user1",
		Seats:    20,
	}
	err = tdb.Storage.BookSeats(ctx, booking1)
	require.NoError(t, err)
	err = tdb.Storage.ConfirmBooking(ctx, event.ID, "user1")
	require.NoError(t, err)

	// Book but don't confirm some seats (should not affect available count)
	booking2 := &models.Booking{
		EventID:  event.ID,
		UserName: "user2",
		Seats:    10,
	}
	err = tdb.Storage.BookSeats(ctx, booking2)
	require.NoError(t, err)

	// Check available seats (should be 80, not counting pending booking)
	available, err = tdb.Storage.GetAvailableSeats(ctx, event.ID)
	require.NoError(t, err)
	assert.Equal(t, 80, available)
}

func TestGetAllEvents(t *testing.T) {
	tdb := setupTestDB(t)
	defer tdb.Cleanup(t)

	ctx := context.Background()

	// Create multiple events
	events := []*models.Event{
		{
			Name:        "Concert",
			Date:        time.Now().Add(24 * time.Hour),
			TotalSeats:  200,
			PaymentTime: 30,
		},
		{
			Name:        "Workshop",
			Date:        time.Now().Add(48 * time.Hour),
			TotalSeats:  50,
			PaymentTime: 15,
		},
		{
			Name:        "Conference",
			Date:        time.Now().Add(72 * time.Hour),
			TotalSeats:  500,
			PaymentTime: 60,
		},
	}

	for _, event := range events {
		err := tdb.Storage.CreateEvent(ctx, event)
		require.NoError(t, err)
	}

	// Get all events
	retrievedEvents, err := tdb.Storage.GetAllEvents(ctx)
	require.NoError(t, err)
	require.Len(t, retrievedEvents, 3)

	// Verify events are returned
	eventNames := make(map[string]bool)
	for _, event := range retrievedEvents {
		eventNames[event.Name] = true
	}
	assert.True(t, eventNames["Concert"])
	assert.True(t, eventNames["Workshop"])
	assert.True(t, eventNames["Conference"])
}

