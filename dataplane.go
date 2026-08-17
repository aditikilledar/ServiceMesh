package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	pb "servicemesh/proto"
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

// Modify your Proxy (Data Plane) so that when it receives a request meant for `http://user-service`
// it queries the Control Plane for the IP/port of `user-service` before forwarding the request.
func (sidecar *Sidecar) CreateDynamicProxy() *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Extract service name
			serviceName := pr.In.Header.Get("Target-Service")
			if serviceName == "" {
				serviceName = sidecar.serviceName
			}

			// ADD TIMEOUT to Query Control Plane via gRPC
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// Dynamically query Control Plane to get LIVE route for that service
			targetUrlStr, err := sidecar.getRouteFromControlPlane(ctx)
			if err != nil {
				log.Printf("PROXY ERROR; failed to get route for service: %s", serviceName)
				return
			}

			targetUrl, err := url.Parse(targetUrlStr)
			if err != nil {
				log.Printf("PROXY ERROR; failed to parse route for service: %s", serviceName)
				return
			}

			pr.SetURL(targetUrl)
			pr.Out.Host = targetUrl.Host
			pr.Out.Header.Set("Mesh-Proxy", "true")
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

func (sidecar *Sidecar) getRouteFromControlPlane(ctx context.Context) (string, error) {
	req := &pb.RouteRequest{
		ServiceName: sidecar.serviceName,
	}

	res, err := sidecar.grpcClient.GetRouting(context.Background(), req)
	if err != nil {
		return "", err
	}

	if !res.GetSuccess() {
		return "", fmt.Errorf("Route not found for service ", sidecar.serviceName)
	}

	return res.GetAddress(), nil
}

func (sidecar *Sidecar) RegisterServiceWithControlPlane(ctx context.Context, address string) error {
	if address == "" {
		return fmt.Errorf("Cannot register empty address for service %s", sidecar.serviceName)
	}

	req := &pb.RegisterRequest{
		ServiceName: sidecar.serviceName,
		Address:     address,
	}

	res, err := sidecar.grpcClient.RegisterService(context.Background(), req)
	if err != nil {
		return err
	}

	if !res.GetSuccess() {
		return fmt.Errorf("Could not register route bro sorry")
	}

	log.Printf("Successfully registered %s:%s", sidecar.serviceName, address)

	sidecar.targetUrl = address

	return nil
}
