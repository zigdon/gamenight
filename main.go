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
	"google.golang.org/api/iterator"
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
	user, err := loggedIn(w, r)
	if err != nil {
		log.Print(err.Error())
		return
	}
	if !user.Superuser {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	ctx := r.Context()
	out := func(t string, args ...any) {
		fmt.Fprintf(w, t+"\n", args...)
	}
	w.Header().Set("Content-Type", "text/html")
	if err := r.ParseForm(); err != nil {
		out("Error parsing form data: %v", err)
	}
	if r.FormValue("delete") == "Submit" {
		t := r.FormValue("type")
		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			out("Can't parse %q: %v", r.FormValue("key"), err)
		}
		key := datastore.IDKey(t, int64(id), nil)
		var gn Gamenight
		err = dsClient.Get(ctx, key, &gn)
		if err != nil {
			out("Error loading %v to delete: %v", key, err)
		} else if err := dsClient.Delete(ctx, key); err != nil {
			out("Error deleting %v: %v", key, err)
		} else {
			out("Deleted %v", key)
		}
	}
	out("<form>Delete:<br/>kind <input name=\"type\"/> ")
	out("id <input name=\"id\"/> ")
	out("<input name=\"delete\" type=\"submit\"/></form><hr/>")
	out("<pre>")
	out("Gamenights:")
	it := dsClient.Run(ctx,
	    datastore.NewQuery("Gamenight").
		FilterField("d", ">", time.Now()).
		Order("-d"))
	for {
		var gn Gamenight
		k, err := it.Next(&gn)
		if err == iterator.Done {
			break
		}
		if err != nil {
			out("iterator error: %v", err)
			continue
		}
		if err := gn.Load(ctx); err != nil {
			log.Printf("error loading gn %v: %v", k, err)
		}
		var id string
		if gn.InviteKey == nil {
			id = "N/A"
		} else {
			id = fmt.Sprintf("%d", gn.InviteKey.ID)
		}
		out("%20d | %s | %s", gn.ID.ID, gn.When(), id)
	}

	invs, err := getAllInvitations(ctx, time.Now().In(tz()))
	if err != nil {
		out("inv query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out("\nInvitations (%d):", len(invs))
	slices.SortFunc(invs, func(a, b Invitation) int {
		return int(b.When().Unix()-a.When().Unix())
	})
	for _, i := range invs {
		if err := i.Load(ctx); err != nil {
			out("Error loading invite: %v", err)
		}
		gnid := int64(-1)
		gn, err := i.GetGamenight(ctx)
		if err != nil {
			out("error getting gn: %v", err)
		}
		if gn != nil {
			gnid = gn.ID.ID
		}
		out("%20d | %s | %15s | %20d | %s: %s", i.Key.ID, i.When(), i.GetOwner().Name, gnid, i.Location, i.Notes)
	}

	it = dsClient.Run(ctx, datastore.NewQuery("User"))
	out("\nUsers:")
	for {
		var u User
		k, err := it.Next(&u)
		if err == iterator.Done {
			break
		}
		if err != nil {
			out("error getting user: %v", err)
			continue
		}
		out("%30s | %20s | %s", k.Name, u.Name, u.DefaultLocation)
	}
}


func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/_ah/logout?continue=/", http.StatusFound)
}

func mkDefault() Gamenight {
	// Before tuesday, say probably. 
	// Before friday, say maybe.
	// Then say no.
	switch (time.Now().In(tz()).Weekday()) {
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
	now := time.Now().In(tz())
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
		sched.Date = sched.Date.In(tz())
		sched.Time = sched.Time.In(tz())
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
	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/index.html"))
	err = tmpl.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		log.Printf("Error executing index: %v", err)
	}
}
