package main

import (
	"fmt"
	"time"

	"cloud.google.com/go/datastore"
)

type Status int

const (
	StatusUndefined Status = iota // 0
	StatusYes                     // 1
	StatusProbably                // 2
	StatusMaybe                   // 3
	StatusNo                      // 4
)

type Priority int

const (
	PriorityUndefined Priority = iota // 0
	PriorityCan                       // 1
	PriorityWant                      // 2
	PriorityInsist                    // 3
)

type User struct {
	ID              string `datastore:"-" json:"id"`
	DefaultLocation string `datastore:"l" json:"location"`
	Superuser       bool   `datastore:"s" json:"superuser"`
	Emails          bool   `datastore:"e" json:"emails"`
	Name            string `datastore:"n" json:"name"`
}

type Gamenight struct {
	ID         int64          `datastore:"-" json:"id"`
	Invite     *datastore.Key `datastore:"a" json:"inviteId"`
	EventID    string         `datastore:"e" json:"calendarId"`
	Status     string         `datastore:"s" json:"status"`
	LastUpdate time.Time      `datastore:"u" json:"lastUpdate"`
	// Denormalized from invitation
	Date     time.Time      `datastore:"d" json:"date"`
	Time     time.Time      `datastore:"t" json:"time"`
	Owner    *datastore.Key `datastore:"o" json:"owner"`
	Location string         `datastore:"l" json:"location"`
	Notes    string         `datastore:"n" json:"notes"`
}

func (g Gamenight) When() time.Time {
	return dateTime(g.Date, g.Time)
}

func (g Gamenight) String() string {
	if g.Owner == nil {
		return fmt.Sprintf("%s: N/A", g.When())
	}
	return fmt.Sprintf("%s: %s@%s - %s", g.When(), g.Owner.Name, g.Location, g.Status)
}

type Invitation struct {
	Key      *datastore.Key `datastore:"-" json:"key"`
	Date     time.Time      `datastore:"d" json:"date"`
	Time     time.Time      `datastore:"t" json:"time"`
	Owner    *datastore.Key `datastore:"o" json:"owner"`
	Location string         `datastore:"l" json:"location"`
	Notes    string         `datastore:"n" json:"notes"`
	Priority Priority       `datastore:"p" json:"priority"`
}

func (i Invitation) When() time.Time {
	return dateTime(i.Date, i.Time)
}

func (i Invitation) PriorityText() string {
	switch i.Priority {
	case PriorityCan:
		return "Can"
	case PriorityWant:
		return "Want"
	case PriorityInsist:
		return "Insist"
	default:
		return ""
	}
}


type Config struct {
	Name  string `datastore:"n" json:"name"`
	Value string `datastore:"v" json:"value"`
}

type Auth struct {
	Credentials string `datastore:"c" json:"-"`
}

func dateTime(d, t time.Time) time.Time {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(fmt.Sprintf("Error loading tz: %v", err))
	}
	return time.Date(
		d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), 0, tz)
}
