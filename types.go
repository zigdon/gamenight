package main

import (
	"context"
	"fmt"
	"log"
	"strings"
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
	ID              *datastore.Key `datastore:"__key__"`
	DefaultLocation string         `datastore:"l"`
	Superuser       bool           `datastore:"s"`
	Emails          bool           `datastore:"e"`
	Notify          bool           `datastore:"f"`
	Name            string         `datastore:"n"`
	Color           string         `datastore:"c"`
}

func (u User) Email() string {
	if strings.Contains(u.ID.String(), ",") {
		return strings.Split(u.ID.String(), ",")[1]
	} else {
		return u.ID.String()
	}
}

type Gamenight struct {
	ID         *datastore.Key `datastore:"__key__"`
	EventID    string         `datastore:"e"`
	Status     string         `datastore:"s"`
	LastUpdate time.Time      `datastore:"u"`
	// Denormalized from invitation
	Date     time.Time      `datastore:"d"`
	Time     time.Time      `datastore:"t"`
	Location string         `datastore:"l"`
	Notes    string         `datastore:"n"`
	Owner    User
	Invite   Invitation

	InviteKey   *datastore.Key `datastore:"a"`
	OwnerKey    *datastore.Key `datastore:"o"`
}

func (g Gamenight) GetOwner() User {
	if g.OwnerKey != nil {
		return g.Owner
	}
	return User{
		Name: "unknown",
	}
}

func (g *Gamenight) Delete(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if g.EventID != "" {
		if err := g.RemoveEvent(ctx); err != nil {
			return fmt.Errorf("Error removing event %s: %v", g.EventID, err)
		}
	}

	return dsClient.Delete(ctx, g.ID)
}

func (g *Gamenight) CreateEvent(ctx context.Context) (string, error) {
	return "Not implemented", fmt.Errorf("Not implemented")
}

func (g *Gamenight) RemoveEvent(ctx context.Context) error {
	return fmt.Errorf("Not implemented")
}

func (g *Gamenight) Load(ctx context.Context) error {
	var o User
	var err error
	if g.OwnerKey != nil {
		if err = dsClient.Get(ctx, g.OwnerKey, &o); err != nil {
			log.Printf("error getting owner %v for %v: %v", g.OwnerKey, g.ID, err)
		}
	}
	g.Owner = o
	if g.InviteKey != nil {
		g.Invite, err = getInvite(ctx, g.InviteKey)
		if err != nil {
			log.Printf("error getting invite %v for %v: %v", g.InviteKey, g.ID, err)
		}
	}
	// Convert the time into localtime, because timezone are the WORST.
	g.Date = g.Date.In(tz())
	g.Time = g.Time.In(tz())
	return nil
}

func (g Gamenight) When() time.Time {
	return dateTime(g.Date, g.Time)
}

func (g Gamenight) String() string {
	return fmt.Sprintf("%s: %s@%s - %s", g.When(), g.GetOwner().Name, g.Location, g.Status)
}

type Invitation struct {
	Key      *datastore.Key `datastore:"__key__"`
	Date     time.Time      `datastore:"d"`
	Time     time.Time      `datastore:"t"`
	Location string         `datastore:"l"`
	Notes    string         `datastore:"n"`
	Priority Priority       `datastore:"p"`
	Owner    User

	OwnerKey    *datastore.Key `datastore:"o"`
}

func (i Invitation) GetOwner() User {
	if i.OwnerKey != nil {
		return i.Owner
	}
	return User{
		Name: "unknown",
	}
}

func (i Invitation) GetGamenight(ctx context.Context) (*Gamenight, error) {
	q := datastore.NewQuery("Gamenight").FilterField("a", "=", i.Key)
	var gns []Gamenight
	ks, err := dsClient.GetAll(ctx, q, &gns)
	if err != nil {
		return nil, fmt.Errorf("can't query for gamenight for %s: %b",
		    i.String(), err)
	}
	if len(ks) == 0 {
		return nil, nil
	}
	return &gns[0], nil
}

func (i *Invitation) Load(ctx context.Context) error {
	var owner User
	if err := dsClient.Get(ctx, i.OwnerKey, &owner); err != nil {
		return fmt.Errorf("error getting owner %v for %v: %v", i.OwnerKey, i.Key, err)
	}
	i.Owner = owner
	// Convert the time into localtime, because timezone are the WORST.
	i.Date = i.Date.In(tz())
	i.Time = i.Time.In(tz())
	return nil
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
	return i.OwnerKey.Equal(u.ID)
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

func (i Invitation) String() string {
	return fmt.Sprintf(
		"%s: %s @ %s (%s): %s",
		i.When(), i.GetOwner().Name, i.Location, i.PriorityText(), i.Notes)
}

type invLoader struct {
	Key          *datastore.Key `datastore:"__key__"`
	Date         time.Time      `datastore:"d"`
	Time         time.Time      `datastore:"t"`
	Location     string         `datastore:"l"`
	Notes        string         `datastore:"n"`
	// TODO: Convert old entries from string to int, so we can remove this hack.
	// Handle either string or int, since we changed how we do this.
	Priority     any            `datastore:"p"`
	DateText     string         `datastore:"datetext"`
	PriorityText string         `datastore:"priority_text"`

	OwnerKey     *datastore.Key `datastore:"o"`
	// unused?
	Owner        *User
}

func (il invLoader) Convert() Invitation {
	i := Invitation{
		Key: il.Key,
		Date: il.Date.In(tz()),
		Time: il.Time.In(tz()),
		OwnerKey: il.OwnerKey,
		Location: il.Location,
		Notes: il.Notes,
	}
	if p, ok := il.Priority.(string); ok {
		i.Priority = PriorityFromText(p)
	} else if p, ok := il.Priority.(int64); ok {
		i.Priority = Priority(p)
	} else {
		log.Printf("Unknown value in priority: %v (%T)", il.Priority, il.Priority)
	}

	return i
}


type Config struct {
	Name  string `datastore:"n"`
	Value string `datastore:"v"`
}

type Auth struct {
	Credentials string `datastore:"c"`
}

func dateTime(d, t time.Time) time.Time {
	return time.Date(
		d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), 0, tz())
}
