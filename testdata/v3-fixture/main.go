package main

import (
	"log"
	"net/http"
)

func main() {
	store := NewStore()
	service := NewService(store)
	handler := NewHandler(service)
	log.Fatal(http.ListenAndServe(":8080", handler))
}
