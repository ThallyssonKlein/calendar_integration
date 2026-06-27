package main

import (
	"log"
	"net/http"

	handler "github.com/thallyssonklein/calendar-integration/mcp-server/api"
)

func main() {
	http.HandleFunc("/", handler.Handler)
	log.Println("mcp-server listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
