package main

import (
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"time"
)

// API Goo

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

var routes = []Route{
	Route{
		"Usage",
		"GET",
		"/api-v1/",
		apiUsage,
	},
	Route{
		"Submit",
		"POST",
		"/api-v1/submit",
		createSubmission,
	},
	Route{
		"ListTargets",
		"GET",
		"/api-v1/targets",
		listTargets,
	},
}

func NewRouter() *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		var handler http.Handler

		handler = route.HandlerFunc
		handler = Logger(handler, route.Name)

		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}

	return router
}

func Logger(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		inner.ServeHTTP(w, r)

		log.Printf(
			"%s\t%s\t%s\t%s",
			r.Method,
			r.RequestURI,
			name,
			time.Since(start),
		)
	})
}
