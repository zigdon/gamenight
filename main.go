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
		fmt.Println("ERROR: environment not loaded")
		return
	}

	var err error
	dsClient, err = datastore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	http.HandleFunc("/", handleIndex)
	//http.HandleFunc("/apiauth", nil)
	//http.HandleFunc("/config", nil)
	//http.HandleFunc("/invite", nil)
	//http.HandleFunc("/profile", nil)
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

func config(ctx context.Context, id string) string {
	cQ := datastore.NewQuery("Config").FilterField("n", "=", id)
	var cfgs []Config
	_, err := dsClient.GetAll(ctx, cQ, &cfgs)
	if err != nil {
		log.Printf("config err: %v", err)
		return ""
	}
	log.Printf("config: %v", cfgs)
	return cfgs[0].Value
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
	log.Printf("sched: %s", sched)

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
	log.Printf("GNs:")
	for _, g := range gns {
		log.Print(g.String())
	}

	var invs []Invitation
	invQ := datastore.NewQuery("Invite").
	    FilterField("d", ">", time.Now()).
		Order("d").
		Limit(4)

	_, err = dsClient.GetAll(ctx, invQ, &invs)
	if err != nil {
		log.Printf("query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Invites: %#v", invs)

	type Future struct {
		Type string
		When time.Time
		Location string
		Owner string
	}

	type Data struct {
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
		if _, ok := found[fmt.Sprintf("%s@%s", inv.Owner.Name, inv.When())]; ok {
			continue
		}
		days = append(days, Future{
			Type: "invite",
			When: inv.When(),
			Location: inv.Location,
			Owner: inv.Owner.Name,
		})
	}

	log.Printf("found: %v", found)

	var data = Data{
		Current: sched,
		Future: days,
		Updated: time.Now(),
		CalendarID: config(ctx, "calendar_id"),
	}
	err = tmpl.ExecuteTemplate(w, "index.html", data)
	log.Printf("data: %#v", data)
	if err != nil {
		log.Printf("Error executing index: %v", err)
	}
}
