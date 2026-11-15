package config

import (
	"fmt"
	"strconv"
	"time"
)

type Notifications struct {
	Batch                   int
	Lease, Poll, MaxBackoff time.Duration
	MaxAttempts             int
}

func LoadNotifications(path string) (Database, Notifications, error) {
	db, e := LoadDatabase(path)
	o := Notifications{Batch: 32, Lease: 30 * time.Second, Poll: time.Second, MaxBackoff: 5 * time.Minute, MaxAttempts: 8}
	if e != nil {
		return db, o, e
	}
	raw, e := loadInput(path)
	if e != nil {
		return db, o, e
	}
	for _, field := range []struct {
		raw   string
		value *int
	}{{raw.NotificationBatch, &o.Batch}, {raw.NotificationAttempts, &o.MaxAttempts}} {
		if field.raw != "" {
			*field.value, e = strconv.Atoi(field.raw)
			if e != nil {
				return db, o, fmt.Errorf("invalid notification integer setting")
			}
		}
	}
	for _, field := range []struct {
		raw   string
		value *time.Duration
	}{{raw.NotificationLease, &o.Lease}, {raw.NotificationPoll, &o.Poll}, {raw.NotificationBackoff, &o.MaxBackoff}} {
		if field.raw != "" {
			*field.value, e = time.ParseDuration(field.raw)
			if e != nil {
				return db, o, fmt.Errorf("invalid notification duration setting")
			}
		}
	}
	return db, o, nil
}
