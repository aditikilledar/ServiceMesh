package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// DATA PLANE = collection of all the proxies / sidecars working together
// FOR MVP: Goal: Write a basic HTTP reverse proxy in Go that sits between a client and a target service.

func (sidecar *Sidecar) CreateReverseProxy(targetUrlStr string) (*httputil.ReverseProxy, error) {
	targetUrl, err := url.Parse(targetUrlStr)
	if err != nil {
		log.Print("bro the targetUrl is invalid")
		return nil, err
	}

	// create reverse proxy instance
	revProxy := &httputil.ReverseProxy{}

	// need to retain query path and logic; else it will throw unrecognized if it's still localhost
	revProxy.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(targetUrl)         // Configures Scheme, Host, and Path routing
		pr.Out.Host = targetUrl.Host // updates outgoing Host header
		pr.Out.Header.Set("Mesh-Proxy", "true")
	}

	return revProxy, nil
}

func (sidecar *Sidecar) CreateDynamicProxy() *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			serviceName := pr.In.Header.Get("Target-Service")

			targetUrlStr := sidecar.controller.getRouteMapping(serviceName)
			targetUrl, _ := url.Parse(targetUrlStr)

			pr.Out.Header.Set("Mesh-Proxy", "true")
			pr.SetURL(targetUrl)
			pr.Out.Host = targetUrl.Host
		},
	}
}

func (sidecar *Sidecar) StartSidecar(wg *sync.WaitGroup) error {
	// mark routine as done before starting server
	if wg != nil {
		wg.Done()
	}

	log.Print("starting Sidecar on port:", sidecar.listenPort)

	// pass proxy as the handler to this server :)
	err := http.ListenAndServe(sidecar.listenPort, sidecar.proxy)
	if err != nil {
		log.Print("Error starting server, ", err)
	}

	return err
}

func main() {
	var waitgroup sync.WaitGroup

	realApplication := &ApplicationServer{
		listenPort: ":8081",
	}
	// tell waitgroup we are waiting for 1 server (application server) to complete initialization
	waitgroup.Add(1)

	go func() {
		err := realApplication.StartAppServer(&waitgroup)
		if err != nil {
			log.Fatal("Error starting Application on ", realApplication.listenPort, " with error: ", err)
		}
	}()
	// currently has no waitgroups - will run only until main() executes

	var targetUrlStr string = "http://localhost:8081"

	// Block main thread until AppServer is Done starting
	waitgroup.Wait()
	log.Print("Application Server running on ", realApplication.listenPort, ". Now starting our sidecar proxy with a targetUrl: ", targetUrlStr)

	// sidecar proxy runs on main thread and blocks
	sidecarProxy := &Sidecar{
		proxy:      nil,
		listenPort: ":8080",
		targetUrl:  targetUrlStr,
	}
	revProxy, err := sidecarProxy.CreateReverseProxy(sidecarProxy.targetUrl)
	if err != nil {
		log.Fatal("Error creating sidecar proxy to targetUrl: ", targetUrlStr)
	} else {
		sidecarProxy.proxy = revProxy
	}
	// pass nil waitgroup bc we don't wanna wait for anything while starting this server
	err = sidecarProxy.StartSidecar(nil)
	if err != nil {
		log.Fatal(err)
	}
}
