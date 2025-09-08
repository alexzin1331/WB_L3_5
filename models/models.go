package models

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port string `yaml:"port"`
	} `yaml:"server"`
	Database struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
	} `yaml:"database"`
}

func MustLoadConfig(path string) *Config {
	file, err := os.Open(path)
	if err != nil {
		panic(fmt.Errorf("open config: %v", err))
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		panic(fmt.Errorf("decode config: %v", err))
	}

	return &cfg
}

type Event struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Date        time.Time `json:"date"`
	TotalSeats  int       `json:"total_seats"`
	PaymentTime int       `json:"payment_time"`
	CreatedAt   time.Time `json:"created_at"`
}

type Booking struct {
	ID        int       `json:"id"`
	EventID   int       `json:"event_id"`
	UserName  string    `json:"user_name"`
	Seats     int       `json:"seats"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
