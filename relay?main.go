package main

import (
	"fmt"
	"Log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Relay server running!")
	})
	log.Println("Relay server started on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
