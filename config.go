package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
)

type ConfigData struct {
	Base BaseTemplate
	Config map[string]string
	Updated map[string]string
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	user, err := loggedIn(w, r)
	if err != nil {
		log.Print(err.Error())
		return
	}
	if !user.Superuser {
		log.Printf("%s is not an admin", user.ID.Name)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data := ConfigData{
		Base: BaseTemplate{
			Subtab: "config",
			Tab: "admin",
			User: user,
		},
		Config: make(map[string]string),
		Updated: make(map[string]string),
	}

	ctx := r.Context()
	it := dsClient.Run(ctx, datastore.NewQuery("Config"))
	config := make(map[*datastore.Key]*Config)
	for {
		var c Config
		k, err := it.Next(&c)
		if err == iterator.Done {
			break
		} else if err != nil {
			log.Printf("Error reading config: %v", err)
			continue
		}
		config[k] = &c
	}

	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/config.html"))
	defer func() {
		if err := tmpl.ExecuteTemplate(w, "config.html", data); err != nil {
			log.Printf("Error execution config.html: %v", err)
		}
	}()

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Base.Error = fmt.Sprintf("Error parsing form: %v", err)
			return
		}
		updated := false
		for k, v := range config {
			nv := r.FormValue("config_"+v.Name)
			if nv != v.Value {
				log.Printf("Config[%s]: %q -> %q", v.Name, v.Value, nv)
				updated = true
				config[k] = &Config{v.Name, nv}
			}
		}
		var newConfig *Config
		if nn := r.FormValue("new_name"); nn != "" {
			newConfig = &Config{
				Name: nn,
				Value: r.FormValue("new_value"),
			}
			log.Printf("Adding new config: %v", newConfig)
			updated = true
		}
		if updated {
			tx, err := dsClient.NewTransaction(ctx)
			if err != nil {
				data.Base.Error = fmt.Sprintf("Error creating transaction: %v", err)
				return
			}
			var keys []*datastore.Key
			var vals []*Config
			for k, v := range config {
				keys = append(keys, k)
				vals = append(vals, v)
			}
			old := make([]*Config, len(keys))
			if err := tx.GetMulti(keys, old); err != nil {
				data.Base.Error = fmt.Sprintf("Error getting old values: %v", err)
				return
			}
			for _, v := range old {
				if v.Value != r.FormValue("config_"+v.Name) {
					data.Updated[v.Name] = "failed"
				}
			}
			if newConfig != nil {
				nk := datastore.IncompleteKey("Config", nil)
				keys = append(keys, nk)
				vals = append(vals, newConfig)
				log.Printf("Adding new key %s for %v", nk, newConfig)
				data.Updated[newConfig.Name] = "failed"
				config[nk] = newConfig
			}
			if _, err = tx.PutMulti(keys, vals); err != nil {
				data.Base.Error = fmt.Sprintf("Error updating values: %v", err)
				return
			}
			if _, err := tx.Commit(); err != nil {
				data.Base.Error = fmt.Sprintf("Error committing transaction: %v", err)
				return
			}
			for k := range data.Updated {
				data.Updated[k] = "yes"
			}
		} else {
			data.Base.Msg = "No changes detected"
		}
	}
	for _, c := range config {
		data.Config[c.Name] = c.Value
	}
}
