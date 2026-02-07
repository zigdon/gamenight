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
		"2006-01-02 3pm",              // 0: YYYY-MM-DD HHpm
		"2006-01-02 15:04",            // 1: YYYY-MM-DD HH:mm
		"2006-01-02",                  // 2: YYYY-MM-DD
		"Jan 2 15:04",                 // 3: mmm DD HH:mm
		"Jan 2",                       // 4: mmm DD
		"1/2 15:04",                   // 5: MM/DD HH:mm
		"1/2 3pm",                     // 6: MM/DD HHpm
		"1/2",                         // 7: MM/DD
	}
	found := false
	for _, layout := range layouts {
		parsedTime, err = time.Parse(layout, timespec)
		if err == nil {
			log.Printf("Parsed date %q as %q: %s", timespec, layout, parsedTime)
			found = true
			break
		}
		log.Printf("Failed to parse date %q as %q: %v", timespec, layout, err)
	}

	if !found {
		return parsedTime, err
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

