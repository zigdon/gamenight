package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
)

func config(ctx context.Context, id string) string {
	cQ := datastore.NewQuery("Config").FilterField("n", "=", id)
	var cfgs []Config
	_, err := dsClient.GetAll(ctx, cQ, &cfgs)
	if err != nil {
		log.Printf("config err: %v", err)
		return ""
	}
	if len(cfgs) > 0 {
		return cfgs[0].Value
	}
	log.Printf("No config found for %v", id)
	return ""
}

func getAllUsers(ctx context.Context) ([]User, error) {
	var users []User
	q := datastore.NewQuery("User")
	_, err := dsClient.GetAll(ctx, q, &users)
	if err != nil {
		return nil, fmt.Errorf("Error querying users: %v", err)
	}
	return users, nil
}

func getUser(ctx context.Context, email string) (*User, error) {
	user := &User{}
	key := datastore.NameKey("User", email, nil)
	err := dsClient.Get(ctx, key, user)
	if err != nil {
		user = &User{
			ID:   key,
			Name: strings.Split(email, "@")[0],
		}
		nk, err := dsClient.Put(ctx, user.ID, user)
        if err != nil {
            log.Printf("Error saving new user to Datastore: %v", err)
            return nil, fmt.Errorf("Couldn't created user")
        }

		log.Printf("New user created: %v", nk)
	}
	if strings.Contains(user.Name, "@") {
		user.Name = strings.Split(user.Name, "@")[0]
	}
	return user, nil
}

func parseTimespec(timespec string) (time.Time, error) {
	var parsedTime time.Time
	var err error
	// Try to parse as specific layout first, then more general ones.
	// Example: "Saturday, 8pm", "Oct 12, 7:30pm"
	layouts := []string{
		"2006-01-02 3pm",
		"2006-01-02, 3pm",
		"2006-01-02 3:04pm",
		"2006-01-02, 3:04pm",
		"2006-01-02 15:04",
		"2006-01-02, 15:04",
		"2006-01-02",
		"Jan 2 3pm",
		"Jan 2, 3pm",
		"Jan 2 3:04pm",
		"Jan 2, 3:04pm",
		"Jan 2 15:04",
		"Jan 2, 15:04",
		"Jan 2",
		"1/2 3pm",
		"1/2, 3pm",
		"1/2 3:04pm",
		"1/2, 3:04pm",
		"1/2 15:04",
		"1/2, 15:04",
		"1/2",
		// from here on we're really reaching (n=21)
		"Monday 3pm",
		"Monday, 3pm",
		"Monday 3:04pm",
		"Monday, 3:04pm",
		"Monday 15:04",
		"Monday, 15:04",
		"Monday",
	}
	found := -1
	for n, layout := range layouts {
		parsedTime, err = time.Parse(layout, timespec)
		if err == nil {
			log.Printf("Parsed date %q as %q: %s", timespec, layout, parsedTime)
			found = n
			break
		}
		log.Printf("Failed to parse date %q as %q: %v", timespec, layout, err)
	}

	if found == -1 {
		return parsedTime, err
	}

	// If all we got was a weekday (and maybe a time), the date will parse
	// as 0000-01-01. In that case, we need to get the weekday requested, and
	// find the next one of those for the date.
	if (found >= 21) {
		req := strings.ToLower(strings.Split(strings.Split(timespec, ",")[0], " ")[0])
		cur := time.Now()
		cnt := 0
		log.Printf("Looking for the next %q", req)
		for strings.ToLower(cur.Weekday().String()) != req {
			if cnt > 7 {
				return parsedTime, fmt.Errorf("Can't figure out what you mean by %q", req)
			}
			cnt++
			cur = cur.AddDate(0, 0, 1)
		}
		log.Printf("Found %s", cur.Format("2006-01-02"))
		parsedTime = parsedTime.AddDate(cur.Year(), int(cur.Month())-1, cur.Day()-1)
		log.Printf("Parsed: %s", parsedTime.String())
	}

	if parsedTime.Hour() == 0 {
		parsedTime = parsedTime.Add(20*time.Hour)
	}

	// Default year is this year unless it's in the past
	now := time.Now()
	if parsedTime.Year() == 0 {
		parsedTime = parsedTime.AddDate(now.Year(), 0, 0)
		if (parsedTime.Before(now)) {
			parsedTime = parsedTime.AddDate(1, 0, 0)
		}
	}
	return parsedTime, err
}

