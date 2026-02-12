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

func PriorityFromText(p string) Priority {
	switch p {
		case "Can": return PriorityCan
		case "Want": return PriorityWant
		case "Insist": return PriorityInsist
		default: return PriorityUndefined
	}
}

func (p Priority) Description() string {
	switch p {
		case PriorityCan: return "Can host"
		case PriorityWant: return "Want to host"
		case PriorityInsist: return "Would really want to host"
		default: return ""
	}
}

type BaseTemplate struct {
	Tab string
	Error string
	Msg string
	User *User
}

type User struct {
	ID              *datastore.Key `datastore:"__key__" json:"id"`
	DefaultLocation string         `datastore:"l" json:"location"`
	Superuser       bool           `datastore:"s" json:"superuser"`
	Emails          bool           `datastore:"e" json:"remind"`
	Notify          bool           `datastore:"f" json:"emails"`
	Name            string         `datastore:"n" json:"name"`
	Color           string         `datastore:"c" json:"color"`
}

type Gamenight struct {
	ID         *datastore.Key `datastore:"__key__" json:"id"`
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
	Key      *datastore.Key `datastore:"__key__" json:"key"`
	Date     time.Time      `datastore:"d" json:"date"`
	Time     time.Time      `datastore:"t" json:"time"`
	Owner    *datastore.Key `datastore:"o" json:"owner"`
	Location string         `datastore:"l" json:"location"`
	Notes    string         `datastore:"n" json:"notes"`
	Priority Priority       `datastore:"p" json:"priority"`
}

func (i Invitation) DateText() string {
	suf := "th"
	switch i.Date.Day() {
		case 1: suf = "st"
		case 2: suf = "nd"
		case 3: suf = "rd"
	}
	return fmt.Sprintf(i.When().Format("Monday, Jan 2%s, 2006 at 3:04 pm"), suf)
}

func (i Invitation) IsOwner(u User) bool {
	return i.Owner.Equal(u.ID)
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
