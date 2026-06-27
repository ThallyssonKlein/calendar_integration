package main

import (
	"log"
	"net/http"

	handler "github.com/thallyssonklein/calendar-integration/crud-api/api"
)

func main() {
	http.HandleFunc("/", handler.Handler)
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
