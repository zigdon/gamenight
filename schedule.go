package main

import (
	"log"
	"html/template"
	"net/http"
	"sort"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
)

type ScheduleDay struct {
	Date string
	Scheduled *Gamenight
	Invitations []*Invitation
}

type ScheduleData struct {
	Base BaseTemplate
	Days []ScheduleDay
}

func handleSchedule(w http.ResponseWriter, r *http.Request) {
	user, err := loggedIn(w, r)
	if err != nil {
		log.Print(err.Error())
		return
	}

    ctx := r.Context()

	data := ScheduleData{
		Base: BaseTemplate{Tab: "schedule", User: user},
	}
	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/schedule.html"))

	// List all the currently scheduled gamenights
	days := make(map[string]ScheduleDay)
	now := time.Now().In(tz())
	it := dsClient.Run(ctx, 
   	    datastore.NewQuery("Gamenight").
		FilterField("d", ">=", now))

	var dayList []string
	for {
		var gn Gamenight
		_, err := it.Next(&gn)
		if err == iterator.Done {
			break
		} else if err != nil {
			log.Printf("Error getting gamenight: %v", err)
			data.Base.Error = "Database error"
			tmpl.ExecuteTemplate(w, "schedule.html", data)
			return
		}
		if err = gn.Load(ctx); err != nil {
			log.Printf("Error filling gamenight: %v", err)
		}
		date := gn.Date.Format("2006-01-02")
		if _, ok := days[date]; !ok {
			days[date] = ScheduleDay{
				Date: gn.Date.Format("Monday, Jan 2, 2006"),
				Scheduled: &gn,
			}
			dayList = append(dayList, date)
		}
	}

	// Get all the pending invitations.
	invs, err := getAllInvitations(ctx, now, false)
	if err != nil {
		log.Printf("Error getting gamenights: %v", err)
		data.Base.Error = "Database error"
		tmpl.ExecuteTemplate(w, "schedule.html", data)
		return
	}
	log.Printf("Loaded %d invitations", len(invs))
	for _, inv := range invs {
		if err = inv.Load(ctx); err != nil {
			log.Printf("Error filling invitation: %v", err)
		}
		date := inv.Date.Format("2006-01-02")
		e, ok := days[date]
		if !ok {
			e = ScheduleDay{
				Date: inv.Date.Format("Monday, Jan 2, 2006"),
				Invitations: []*Invitation{},
			}
			dayList = append(dayList, date)
		}
		if days[date].Scheduled == nil || !days[date].Scheduled.Invite.Key.Equal(inv.Key) {
			e.Invitations = append(e.Invitations, inv)
		}
		days[date] = e
	}
	sort.Strings(dayList)
	for _, k := range dayList {
		data.Days = append(data.Days, days[k])
	}

	err = tmpl.ExecuteTemplate(w, "schedule.html", data)
	if err != nil {
		log.Printf("Error executing schedule.html: %v", err)
	}
}
