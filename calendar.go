package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type calSvc struct {
	*calendar.Service
	calendarID string
}

func newCalSvc(ctx context.Context) (*calSvc, error) {
	srv, err := calendar.NewService(
		ctx, option.WithCredentialsJSON([]byte(config(ctx, "service_account"))))
	if err != nil {
		return nil, fmt.Errorf("Error connecting to calendar: %v", err)
	}
	return &calSvc{
		srv, config(ctx, "calendar_id"),
	}, nil
}

func (s *calSvc) Add(ctx context.Context, when time.Time, location, notes string, invite []User) (string, error) {
	// TODO: remove this when testing is done
	when = when.AddDate(-1, 0, 0)

	// Make sure we're in the correct timezone. Because stupid timezones.
	when = when.In(tz())

	midnight := time.Date(when.Year(), when.Month(), when.Day(), 0, 0, 0, 0, tz()).
	    AddDate(0, 0, 1).
		Format(time.RFC3339)
	var invs []*calendar.EventAttendee
	for _, u := range invite {
		invs = append(invs, &calendar.EventAttendee{
			DisplayName: u.Name,
			Email: u.Email(),
		})
	}
	e := &calendar.Event{
		AttendeesOmitted: true,
		Attendees: invs,
		Creator: &calendar.EventCreator{
			DisplayName: "Gamenight!",
			Self: true,
		},
		Description: notes,
		End: &calendar.EventDateTime{
			DateTime: midnight,
			TimeZone: tz().String(),
		},
		Location: location,
		Start: &calendar.EventDateTime{
			DateTime: when.Format(time.RFC3339),
			TimeZone: tz().String(),
		},
		Summary: "(test) Gamenight: YES",
	}
	event, err := s.Events.Insert(s.calendarID, e).Do()
	if err != nil {
		log.Printf("Error creating event %#v: %v", e, err)
		return "", fmt.Errorf("Failed to create event: %v", err)
	}
	log.Printf("Event created: %#v", event)

	return event.Id, nil
}

func (s *calSvc) Get(ctx context.Context, eid string) (*calendar.Event, error) {
	return s.Events.Get(s.calendarID, eid).Do()
}

func (s *calSvc) Remove(ctx context.Context, eid string) error {
	return s.Events.Delete(s.calendarID, eid).SendUpdates("all").Do()
}
