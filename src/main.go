package main

import (
	"net/http"
)

func main() {
	loadEnvironmentVariables()
	http.HandleFunc("/", routeHandler)

	http.ListenAndServe(":8090", nil)
}
