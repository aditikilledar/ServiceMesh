package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// DATA PLANE = collection of all the proxies / sidecars working together
// FOR MVP: Goal: Write a basic HTTP reverse proxy in Go that sits between a client and a target service.

// 1. Create a HTTP server that listens in on a port - 8080
// 2. Create an endpoint to accept an incoming request and forward it to a hardcoded URL
func (s *Server) Home(rw http.ResponseWriter, r *http.Request) {
	// business logic to handle when this endpoint is hit
	// fmt.Fprintf(rw, "Hello World :)") <--- THIS BREAKS IT BC
	// Go automatically sets the HTTP status code to 200 OK and sends "Hello World :)" right down the wire to the browser. Once this happens, the HTTP headers are permanently sent and locked AND SO PROXY GETS DAMN CONFUSED

	if s.revProxy != nil {
		log.Print(s.portNumber, ": Proxying to Hello World :P")
		s.revProxy.ServeHTTP(rw, r)
	} else {
		log.Print(s.portNumber, ": I am Hello World :P")
		fmt.Fprintf(rw, "Hello World :) from %s", s.portNumber)
	}
}

func (s *Server) Info(rw http.ResponseWriter, r *http.Request) {
	// business logic to handle when this endpoint is hit
	fmt.Fprintf(rw, "Hi I am reporting something about myself!!!!!")
	log.Print("In INFO")
}

func (server *Server) StartServer(handler ...http.Handler) error {
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

	log.Print("starting server on port:", server.portNumber)

	err := http.ListenAndServe(server.portNumber, resolvedHandler)
	if err != nil {
		log.Print("Error starting server, ", err)
	}

	return err
}

func CreateReverseProxy(targetUrlStr string) *httputil.ReverseProxy {
	targetUrl, err := url.Parse(targetUrlStr)
	if err != nil {
		log.Print("bro the targetUrl is invalid")
	}

	// create reverse proxy instance
	revProxy := &httputil.ReverseProxy{}

	// need to retain query path and logic; else it will throw unrecognized if it's still localhost
	revProxy.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(targetUrl)         // Configures Scheme, Host, and Path routing
		pr.Out.Host = targetUrl.Host // updates outgoing Host header
		pr.Out.Header.Set("Mesh-Proxy", "true")
	}

	return revProxy
}

func main() {
	realApplication := &Server{
		portNumber: ":8081",
	}
	go realApplication.StartServer()
	// currently has no waitgroups - will run only until main() executes

	var targetUrlStr string = "http://localhost:8081"

	// server A
	sidecarProxy := &Server{
		revProxy:   CreateReverseProxy(targetUrlStr),
		portNumber: ":8080",
	}
	err := sidecarProxy.StartServer()
	if err != nil {
		log.Print(err)
	}
}
