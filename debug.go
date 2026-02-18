package main

import(
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
)

func handleDebug(w http.ResponseWriter, r *http.Request) {
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
		out("%20d | %s | %10s | %s", gn.ID.ID, gn.When(), id, gn.EventID)
	}

	invs, err := getAllInvitations(ctx, time.Now(), false)
	if err != nil {
		out("inv query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out("\nInvitations (%d):", len(invs))
	slices.SortFunc(invs, func(a, b *Invitation) int {
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

