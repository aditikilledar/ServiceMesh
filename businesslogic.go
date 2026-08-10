package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

func (s *ApplicationServer) Home(rw http.ResponseWriter, r *http.Request) {
	// business logic to handle when this endpoint is hit

	log.Print(s.listenPort, ": Application Business Logic Reached")
	fmt.Fprintf(rw, "Hello World :P \n from %s", s.listenPort)
}

func (s *ApplicationServer) Info(rw http.ResponseWriter, r *http.Request) {
	// business logic to handle when this endpoint is hit
	fmt.Fprintf(rw, "Hi I am reporting something about myself!!!!!")
	log.Print("In INFO")
}

func (server *ApplicationServer) StartAppServer(wg *sync.WaitGroup, handler ...http.Handler) error {
	var resolvedHandler http.Handler
	// ... makes it a slice/array
	if len(handler) > 0 {
		resolvedHandler = handler[0]
	} else {
		// 1. Create an isolated, local multiplexer for this server instance
		mux := http.NewServeMux()

		// 2. Register routes directly to this local mux instead of the global http package
		mux.HandleFunc("/", server.Home)
		mux.HandleFunc("/info", server.Info)

		resolvedHandler = mux
	}

	// mark routine as done before starting server
	if wg != nil {
		wg.Done()
	}

	log.Print("starting server on port:", server.listenPort)

	err := http.ListenAndServe(server.listenPort, resolvedHandler)
	if err != nil {
		log.Print("Error starting server, ", err)
	}

	return err
}
