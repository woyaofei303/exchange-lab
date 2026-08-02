package main

import (
	"fmt"
	"net/http"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "exchange-lab is running")
}

func main() {

	http.HandleFunc("/health", healthHandler)

	fmt.Println("server started :8080")

	http.ListenAndServe(":8080", nil)
}
