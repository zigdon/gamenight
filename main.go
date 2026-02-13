package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"cloud.google.com/go/datastore"
)

var (
	dsClient *datastore.Client
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
	http.HandleFunc("/schedule", handleSchedule)
	//http.HandleFunc("/tasks/nag", nil)
	//http.HandleFunc("/tasks/reset", nil)
	http.HandleFunc("/tasks/schedule", handleTaskSchedule)

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
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/html")
	if err := r.ParseForm(); err != nil {
		fmt.Fprintf(w, "Error parsing form data: %v", err)
	}
	if r.FormValue("delete") == "Submit" {
		t := r.FormValue("type")
		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			fmt.Fprintf(w, "Can't parse %q: %v", r.FormValue("key"), err)
		}
		key := datastore.IDKey(t, int64(id), nil)
		var gn Gamenight
		err = dsClient.Get(ctx, key, &gn)
		if err != nil {
			fmt.Fprintf(w, "Error loading %v to delete: %v", key, err)
		} else if err := dsClient.Delete(ctx, key); err != nil {
			fmt.Fprintf(w, "Error deleting %v: %v", key, err)
		} else {
			fmt.Fprintf(w, "Deleted %v", key)
		}
	}
	fmt.Fprintf(w, "<form>Delete:<br/>kind <input name=\"type\"/> ")
	fmt.Fprintf(w, "id <input name=\"id\"/> ")
	fmt.Fprintf(w, "<input name=\"delete\" type=\"submit\"/></form><hr/>")
	fmt.Fprintf(w, "<pre>")
	fmt.Fprintf(w, "Gamenights:\n")
	var gns []Gamenight
	_, err := dsClient.GetAll(ctx, datastore.NewQuery("Gamenight").Order("-d"), &gns)
	if err != nil {
		log.Printf("gn query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, gn := range gns {
		if err := gn.Load(ctx); err != nil {
			log.Printf("error loading gn %v: %v", gn.ID, err)
		}
		var id string
		if gn.InviteKey == nil {
			id = "N/A"
		} else {
			id = fmt.Sprintf("%d", gn.InviteKey.ID)
		}
		fmt.Fprintf(w, "%20d | %s | %s\n", gn.ID.ID, gn.When(), id)
	}

	fmt.Fprintf(w, "Invitations:\n")
	invs, err := getAllInvitations(ctx, time.Unix(0, 0))
	if err != nil {
		log.Printf("inv query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slices.SortFunc(invs, func(a, b Invitation) int {
		return int(b.When().Unix()-a.When().Unix())
	})
	for _, i := range invs {
		if err := i.Load(ctx); err != nil {
			fmt.Fprintf(w, "Error loading invite: %v", err)
		}
		fmt.Fprintf(w, "%20d | %s | %15s | %s: %s\n", i.Key.ID, i.When(), i.GetOwner().Name, i.Location, i.Notes)
	}

	userQ := datastore.NewQuery("User")

	var users []User
	_, err = dsClient.GetAll(ctx, userQ, &users)
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
	now := time.Now()
	found := make(map[string]bool)
	schQ := datastore.NewQuery("Gamenight").
		FilterField("s", "=", "Yes").
		FilterField("d", ">", now.AddDate(0,0,-2)).
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
		if err := sched.Load(ctx); err != nil {
			log.Printf("Error loading %s: %v", sched.ID, err)
		}
		found[fmt.Sprintf("%s@%s", sched.GetOwner().Name, sched.When())] = true
	}

	// Midnight on this saturday.
	sat := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0,0,7-now.Day())
	gnQ := datastore.NewQuery("Gamenight").
		FilterField("d", "<", sat).
		FilterField("d", ">=", now).
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
	    FilterField("d", ">", now).
	    FilterField("d", "<", now.AddDate(0, 0, 7)).
		Order("d")

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
		if err = gn.Load(ctx); err != nil {
			log.Printf("Error filling gn: %v", err)
		}
		if (gn.ID.Equal(sched.ID)) {
			continue
		}
		days = append(days, Future{
			Type: "gamenight",
			When: gn.When(),
			Location: gn.Location,
			Owner: gn.GetOwner().Name,
		})
		found[fmt.Sprintf("%s@%s", gn.GetOwner().Name, gn.When())] = true
	}
	for _, inv := range invs {
		if err = inv.Load(ctx); err != nil {
			log.Printf("Error filling inv: %v", err)
		}
		if _, ok := found[fmt.Sprintf("%s@%s", inv.GetOwner().Name, inv.When())]; ok {
			continue
		}
		days = append(days, Future{
			Type: "invite",
			When: inv.When(),
			Location: inv.Location,
			Owner: inv.GetOwner().Name,
		})
	}

	var data = IndexData{
		Current: sched,
		Future: days,
		Updated: now,
		CalendarID: config(ctx, "calendar_id"),
	}
	log.Printf("Current: %#v", sched)
	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/index.html"))
	err = tmpl.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		log.Printf("Error executing index: %v", err)
	}
}
