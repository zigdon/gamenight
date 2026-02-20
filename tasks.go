package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func handleNag(w http.ResponseWriter, r *http.Request) {
	// Whatever happens, redirect back to root.
	defer func() {
		http.Redirect(w, r, "/", http.StatusFound)
	}()

	ctx := r.Context()
	if nextGN, err := getNextGamenight(ctx); err != nil {
		log.Printf("Couldn't find next gamenight: %v", err)
	} else if nextGN != nil {
		now := time.Now().In(tz())
		sun := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz()).AddDate(0,0,int(7-now.Weekday()))
		if nextGN.When().Before(sun) {
			log.Printf("No need to nag, gn is scheduled for %s", nextGN.When())
			return
		}
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		return
	}

	seq := r.FormValue("email")
	var err error
	switch seq {
	case "first":
		err = email(ctx, firstNag, nil)
	case "second":
		err = email(ctx, secondNag, nil)
	default:
		err = fmt.Errorf("Bad nag call: %q", seq)
	}
	if err != nil {
		log.Printf("Error in nag task: %v", err)
	}
}

func handleTaskSchedule(w http.ResponseWriter, r *http.Request) {
	// Whatever happens, redirect back to the schedule page.
	defer func() {
		http.Redirect(w, r, "/schedule", http.StatusFound)
	}()

	// Check auth.
	ctx := r.Context()
	user, _ := getUserSession(ctx, r)

	if _, err := maybeSchedule(ctx, w, user.Superuser); err != nil {
		log.Printf("Error scheduling gamenight: %v", err)
	}
}
