package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/datastore"
)

var (
	dsClient *datastore.Client
	tmpl     = template.Must(template.ParseGlob("templates/*.html"))
)

func main() {
	ctx := context.Background()
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Printf("ERROR: environment variable GOOGLE_CLOUD_PROJECT not set. Datastore client may not initialize correctly.")
		// We can try to proceed without it, but Datastore operations might fail.
	} else {
        log.Printf("Using Google Cloud Project ID: %s", projectID)
    }

	var err error
	dsClient, err = datastore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
    log.Printf("Datastore client initialized successfully.")

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/invite", handleInvite)
	http.HandleFunc("/profile", handleProfile)
	http.HandleFunc("/debug", handleDebug)
	//http.HandleFunc("/schedule", nil)
	//http.HandleFunc("/tasks/nag", nil)
	//http.HandleFunc("/tasks/reset", nil)
	//http.HandleFunc("/tasks/schedule", nil)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleDebug(w http.ResponseWriter, r *http.Request) {
	invQ := datastore.NewQuery("Invitation").Order("d")

	var invs []Invitation
	keys, err := dsClient.GetAll(r.Context(), invQ, &invs)
	if err != nil {
		log.Printf("inv query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Invitations:\n")
	for n, i := range invs {
		fmt.Fprintf(w, "%#v: %#v\n", keys[n],  i)
	}

	userQ := datastore.NewQuery("User")

	var users []User
	keys, err = dsClient.GetAll(r.Context(), userQ, &users)
	if err != nil {
		log.Printf("user query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "\nUsers:\n")
	for _, i := range users {
		fmt.Fprintf(w, "%#v: %#v\n", i.ID, i)
	}
}


func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/_ah/logout?continue=/", http.StatusFound)
}

func mkDefault() Gamenight {
	// Before tuesday, say probably. 
	// Before friday, say maybe.
	// Then say no.
	switch (time.Now().Weekday()) {
	case time.Sunday, time.Monday, time.Tuesday:
		return Gamenight{Status: "Probably"}
	case time.Wednesday, time.Thursday, time.Friday:
		return Gamenight{Status: "Maybe"}
	default:
		return Gamenight{Status: "No"}
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var sched Gamenight
	var gns []Gamenight
	found := make(map[string]bool)
	schQ := datastore.NewQuery("Gamenight").
		FilterField("s", "=", "Yes").
		FilterField("d", ">", time.Now().AddDate(0,0,-2)).
		Order("d")

	_, err := dsClient.GetAll(ctx, schQ, &gns)
	if err != nil {
		log.Printf("sched query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if (len(gns) == 0) {
		sched = mkDefault()
	} else {
		sched = gns[0]
		found[fmt.Sprintf("%s@%s", sched.Owner.Name, sched.When())] = true
	}

	gnQ := datastore.NewQuery("Gamenight").
		FilterField("d", ">", time.Now().AddDate(0,0,-2)).
		Order("d")

	gns = []Gamenight{}
	_, err = dsClient.GetAll(ctx, gnQ, &gns)
	if err != nil {
		log.Printf("gns query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var invs []Invitation
	invQ := datastore.NewQuery("Invitation").
	    FilterField("d", ">", time.Now()).
		Order("d").
		Limit(4)

	_, err = dsClient.GetAll(ctx, invQ, &invs)
	if err != nil {
		log.Printf("query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Future struct {
		Type string
		When time.Time
		Location string
		Owner string
	}

	type IndexData struct {
		Current Gamenight
		Future []Future
		Updated time.Time
		CalendarID string
	}

	var days []Future
	for _, gn := range gns {
		if (gn.ID == sched.ID) {
			continue
		}
		o := "N/A"
		if gn.Owner != nil {
			o = gn.Owner.Name
		}
		days = append(days, Future{
			Type: "gamenight",
			When: gn.When(),
			Location: gn.Location,
			Owner: o,
		})
		if gn.Invite != nil {
			found[fmt.Sprintf("%s@%s", gn.Owner.Name, gn.When())] = true
		}
	}
	for _, inv := range invs {
		name := "N/A"
		if inv.Owner != nil {
			if _, ok := found[fmt.Sprintf("%s@%s", inv.Owner.Name, inv.When())]; ok {
				continue
			}
			name = inv.Owner.Name
		}
		days = append(days, Future{
			Type: "invite",
			When: inv.When(),
			Location: inv.Location,
			Owner: name,
		})
	}

	var data = IndexData{
		Current: sched,
		Future: days,
		Updated: time.Now(),
		CalendarID: config(ctx, "calendar_id"),
	}
	err = tmpl.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		log.Printf("Error executing index: %v", err)
	}
}
