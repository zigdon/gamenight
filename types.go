package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/iterator"
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
	case "Can":
		return PriorityCan
	case "Want":
		return PriorityWant
	case "Insist":
		return PriorityInsist
	default:
		return PriorityUndefined
	}
}

func (p Priority) Description() string {
	switch p {
	case PriorityCan:
		return "Can host"
	case PriorityWant:
		return "Want to host"
	case PriorityInsist:
		return "Would really want to host"
	default:
		return ""
	}
}

type BaseTemplate struct {
	Tab       string
	Subtab    string
	Error     string
	Msg       string
	User      *User
	DevServer bool
}

type userPreference string

const (
	emailsPreference userPreference = "e"
	notifyPreference userPreference = "f"
	invitePreference userPreference = "i"
)

type User struct {
	ID              *datastore.Key `datastore:"__key__"`
	DefaultLocation string         `datastore:"l"`
	Superuser       bool           `datastore:"s"`
	// Get nag emails when a gn has not been scheduled
	Emails bool `datastore:"e"`
	// Get an email when a new gn is scheduled
	Notify bool   `datastore:"f"`
	Invite bool   `datastore:"i"`
	Name   string `datastore:"n"`
	Color  string `datastore:"c"`
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
	Date     time.Time `datastore:"d"`
	Time     time.Time `datastore:"t"`
	Location string    `datastore:"l"`
	Notes    string    `datastore:"n"`
	Owner    *User
	Invite   *Invitation

	InviteKey *datastore.Key `datastore:"a"`
	OwnerKey  *datastore.Key `datastore:"o"`

	EventDetails *calendar.Event `datastore:"-"`
	Relative     string          `datastore:"-"`
}

func (g Gamenight) GetOwner() *User {
	if g.OwnerKey != nil {
		return g.Owner
	}
	return &User{
		Name: "unknown",
	}
}

func (g *Gamenight) Delete(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if g.EventID == "" {
		return fmt.Errorf("No event ID found to remove!")
	} else if err := g.RemoveEvent(ctx); err != nil {
		return fmt.Errorf("Error removing event %s: %v", g.EventID, err)
	}

	return dsClient.Delete(ctx, g.ID)
}

func (g *Gamenight) CreateEvent(ctx context.Context) error {
	users, err := getSelectedUsers(ctx, invitePreference)
	if err != nil {
		log.Printf("Couldn't get users to invite: %v", err)
	}
	eid, err := svc.Add(ctx, g.Date.In(tz()), g.Location, g.Notes, users)
	if err != nil {
		return err
	}
	g.EventID = eid
	if err := g.Save(ctx); err != nil {
		return fmt.Errorf("Failed to update gamenight: %v", err)
	}

	return nil
}

func (g *Gamenight) RemoveEvent(ctx context.Context) error {
	if err := svc.Remove(ctx, g.EventID); err != nil {
		return err
	}
	g.EventID = ""
	return nil
}

func (g *Gamenight) Save(ctx context.Context) error {
	m := datastore.NewUpdate(g.ID, g)
	_, err := dsClient.Mutate(ctx, m)
	return err
}

func (g *Gamenight) Load(ctx context.Context) error {
	var o User
	var err error
	if g.OwnerKey != nil {
		if err = dsClient.Get(ctx, g.OwnerKey, &o); err != nil {
			log.Printf("error getting owner %v for %v: %v", g.OwnerKey, g.ID, err)
		}
	}
	g.Owner = &o
	if g.InviteKey != nil {
		g.Invite, err = getInvite(ctx, g.InviteKey)
		if err != nil {
			log.Printf("error getting invite %v for %v: %v", g.InviteKey, g.ID, err)
		}
	}
	if g.EventID != "" {
		g.EventDetails, err = svc.Get(ctx, g.EventID)
		if err != nil {
			log.Printf("Error getting event %q: %v", g.EventID, err)
		}
	}

	// Convert the time into localtime, because timezone are the WORST.
	g.Date = g.Date.In(tz())
	g.Time = g.Time.In(tz())
	return nil
}

func (g Gamenight) When() time.Time {
	return dateTime(g.Date.In(tz()), g.Time.In(tz()))
}

func (g *Gamenight) String() string {
	if g == nil {
		return "Deleted gamenight"
	}
	name := "N/A"
	owner := g.GetOwner()
	if owner != nil {
		name = owner.Name
	}
	return fmt.Sprintf("%s: %s@%s - %s (%s)",
		g.When(), name, g.Location, g.Status, g.EventID)
}

type Invitation struct {
	Key      *datastore.Key `datastore:"__key__"`
	Date     time.Time      `datastore:"d"`
	Time     time.Time      `datastore:"t"`
	Location string         `datastore:"l"`
	Notes    string         `datastore:"n"`
	Priority Priority       `datastore:"p"`
	Owner    *User

	OwnerKey  *datastore.Key `datastore:"o"`
	Scheduled *Gamenight     `datastore:"-"`
	Relative  string         `datastore:"-"`
}

func (i *Invitation) Schedule(ctx context.Context) error {
	out := log.Printf
	now := time.Now().In(tz())

	// TODO: If there's a tie in date/priority, prefer the person who hasn't
	// hosted recently. For now, let's assume that's not a problem.

	// If the invite is not for Saturday, schedule it.
	// If it is for Saturday, and it's high priority, or today is at least Tuesday, schedule it.
	if i.Date.Weekday() == time.Saturday {
		if i.Priority == PriorityCan && now.Weekday() < time.Tuesday {
			out("Today is %s, not scheduling 'Can' for Saturday yet", now.Weekday())
			return fmt.Errorf("Not scheduling 'can' on %s", now.Weekday())
		}
	}

	out("Scheduling %s", i.String())

	gn := &Gamenight{
		ID:         datastore.IncompleteKey("Gamenight", nil),
		Status:     "Yes",
		LastUpdate: now,
		Date:       i.When(),
		Time:       i.When(), // Redundant and obsolete, but keep filling it for now.
		Location:   i.Location,
		Notes:      i.Notes,
		OwnerKey:   i.OwnerKey,
		InviteKey:  i.Key,
	}

	nk, err := dsClient.Put(ctx, gn.ID, gn)
	if err != nil {
		return fmt.Errorf("Failed to save gamenight: %v", err)
	}

	gn.ID = nk
	i.Scheduled = gn
	out("Created entry: %v", nk)
	if err := i.Save(ctx); err != nil {
		out("Failed to update invitation: %v", err)
	}

	if err := gn.CreateEvent(ctx); err != nil {
		out("Failed to create new event: %v", err)
	}

	if err := gn.Load(ctx); err != nil {
		out("Error loading gn: %v", err)
	} else if err := email(ctx, gnScheduled, gn); err != nil {
		out("Failed to send out notification: %v", err)
	}
	return nil
}

func (i *Invitation) Save(ctx context.Context) error {
	m := datastore.NewUpdate(i.Key, i)
	_, err := dsClient.Mutate(ctx, m)
	return err
}

func (i Invitation) GetOwner() *User {
	if i.OwnerKey != nil {
		return i.Owner
	}
	return &User{
		Name: "unknown",
	}
}

func (i Invitation) GetGamenight(ctx context.Context) (*Gamenight, error) {
	it := dsClient.Run(ctx,
		datastore.NewQuery("Gamenight").FilterField("a", "=", i.Key))
	var gn Gamenight
	_, err := it.Next(&gn)
	if err == iterator.Done {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("can't query for gamenight for %s: %v",
			i.String(), err)
	}
	r := &gn
	return r, nil
}

func (i *Invitation) Load(ctx context.Context) error {
	var owner User
	if err := dsClient.Get(ctx, i.OwnerKey, &owner); err != nil {
		return fmt.Errorf("error getting owner %v for %v: %v", i.OwnerKey, i.Key, err)
	}
	i.Owner = &owner
	// Preload the scheduled gamenight, if any, YOLO.
	i.Scheduled, _ = i.GetGamenight(ctx)
	// Convert the time into localtime, because timezone are the WORST.
	i.Date = i.Date.In(tz())
	i.Time = i.Time.In(tz())
	// Calculate the relative date.
	sameDay := func(a, b time.Time) bool {
		return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
	}
	now := time.Now().In(tz())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz())
	tomorrow := today.AddDate(0, 0, 1)
	if sameDay(i.Date, today) {
		i.Relative = "Today"
	} else if sameDay(i.Date, tomorrow) {
		i.Relative = "Tomorrow"
	} else {
		delta := time.Until(i.Date).Hours() / 24
		i.Relative = fmt.Sprintf("In %d days", int(delta))
	}
	return nil
}

func (i Invitation) DateText() string {
	suf := "th"
	switch i.Date.Day() {
	case 1, 21, 31:
		suf = "st"
	case 2, 22:
		suf = "nd"
	case 3, 23:
		suf = "rd"
	}
	return fmt.Sprintf(i.When().Format("Monday, Jan 2%s, 2006 at 3:04 pm"), suf)
}

func (i Invitation) IsOwner(u User) bool {
	return i.OwnerKey.Equal(u.ID)
}

func (i Invitation) When() time.Time {
	return dateTime(i.Date.In(tz()), i.Time.In(tz()))
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

func (i *Invitation) String() string {
	name := "N/A"
	owner := i.GetOwner()
	if owner != nil {
		name = owner.Name
	}
	return fmt.Sprintf(
		"%s: %s @ %s (%s): %s",
		i.When(), name, i.Location, i.PriorityText(), i.Notes)
}

type invLoader struct {
	Key      *datastore.Key `datastore:"__key__"`
	Date     time.Time      `datastore:"d"`
	Time     time.Time      `datastore:"t"`
	Location string         `datastore:"l"`
	Notes    string         `datastore:"n"`
	// TODO: Convert old entries from string to int, so we can remove this hack.
	// Handle either string or int, since we changed how we do this.
	Priority     any    `datastore:"p"`
	DateText     string `datastore:"datetext"`
	PriorityText string `datastore:"priority_text"`

	OwnerKey *datastore.Key `datastore:"o"`
	// unused?
	Owner     *User
	Scheduled *Gamenight
	Relative  string
}

func (il invLoader) Convert() Invitation {
	i := Invitation{
		Key:      il.Key,
		Date:     il.Date.In(tz()),
		Time:     il.Time.In(tz()),
		OwnerKey: il.OwnerKey,
		Location: il.Location,
		Notes:    il.Notes,
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
	Value string `datastore:"v,noindex"`
}

type Auth struct {
	Credentials string `datastore:"c"`
}

func dateTime(d, t time.Time) time.Time {
	return time.Date(
		d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), 0, tz())
}
