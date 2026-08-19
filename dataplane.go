package main

import (
	"context"
	"fmt"
	"io"
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

func NewSidecar(serviceName string, listenPort string, client pb.ControlPlaneServiceClient) (sidecar *Sidecar) {
	sidecar = &Sidecar{
		serviceName: serviceName,
		listenPort:  listenPort,
		grpcClient:  client,
		endpoints:   make(map[string][]string),
		lbIndices:   make(map[string]*uint64),
	}
	sidecar.proxy = sidecar.CreateDynamicProxy()
	return sidecar
}

// -------------------------------------------------------------------------
// V2: StartRouteStream opens a long-lived gRPC server-stream to the Control
// Plane. It continuously receives pushed endpoint updates and updates local cache.
// -------------------------------------------------------------------------
func (s *Sidecar) StartRouteStream(ctx context.Context, targetService string) {
	// 1. Open long lived gRPC server stream
	stream, err := s.grpcClient.StreamRoutes(ctx, &pb.RouteRequest{
		ServiceName: targetService,
	})
	if err != nil {
		log.Printf("Failed to open route stream for %s:%v", targetService, err)
		return
	}

	// 2. Consume pushed events onto the stream in a dedicated goroutine
	go func() {
		for {
			// block until control plane pushes something onto the stream
			res, err := stream.Recv()
			if err == io.EOF {
				log.Printf("Route stream closed by Control Plane for %s", targetService)
				return
			}
			if err != nil {
				log.Printf("Error reading route stream for %s: %v", targetService, err)
				return
			}

			// 3. Update LOCAL CACHE with newly recieved address/endpoint list from control plane
			if res.GetSuccess() {
				s.updateLocalCache(targetService, res.GetAddresses())
			}
		}
	}()
}

// -------------------------------------------------------------------------
// V2: updateLocalCache updates the in-memory endpoint slice and ensures
// a Round-Robin atomic counter exists for the given service.
// -------------------------------------------------------------------------
func (s *Sidecar) updateLocalCache(targetService string, addresses []string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.endpoints[targetService] = addresses

	// if no counter exists for this service, allocate a new atomic counter
	if _, exists := s.lbIndices[targetService]; !exists {
		var idx uint64 = 0
		s.lbIndices[targetService] = &idx
	}
	log.Printf("[DATA PLANE SIDECAR CACHE UPDATED] Service: %s -> Instances: %v", targetService, addresses)
}

// TODO:
// -------------------------------------------------------------------------
// V2: getNextEndpointLocal fetches an endpoint from local memory using
// atomic Round-Robin modulo arithmetic. Executed in nanoseconds!
// -------------------------------------------------------------------------

// -------------------------------------------------------------------------
// V2 CHANGED: CreateDynamicProxy now routes traffic via local memory cache
// rather than issuing a network call to the Control Plane per request.
// -------------------------------------------------------------------------

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
		// ✅ Intercepts failures when pr.SetURL was never called
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, fmt.Sprintf("Service Mesh Proxy Error: %v", err), http.StatusBadGateway)
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

	res, err := sidecar.grpcClient.GetRouting(ctx, req)
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
