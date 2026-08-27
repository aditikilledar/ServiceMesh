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
	"sync/atomic"
	"time"

	pb "servicemesh/proto"
)

// DATA PLANE = collection of all the proxies / sidecars working together
// FOR MVP: Goal: Write a basic HTTP reverse proxy in Go that sits between a client and a target service.

func NewSidecar(serviceName string, listenPort string, client pb.ControlPlaneClient) *Sidecar {
	sidecar := &Sidecar{
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
func (s *Sidecar) getNextEndpointFromLocal(serviceName string) (string, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	addresses, exists := s.endpoints[serviceName]
	indexPtr := s.lbIndices[serviceName]

	// returm false if no endpoints are cached yet
	if !exists || len(addresses) == 0 {
		return "", false
	}

	// lock free atomic increment across concurrent requests
	nextIndex := atomic.AddUint64(indexPtr, 1)

	// pick target backend using modulo Round Robin Arithmetic
	// subtract 1 to adjust for 0-indexing
	index := (nextIndex - 1) % uint64(len(addresses))
	return addresses[index], true
}

// -------------------------------------------------------------------------
// V2 CHANGED: CreateDynamicProxy now routes traffic via local memory cache
// rather than issuing a network call to the Control Plane per request.
// -------------------------------------------------------------------------

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

			var found bool
			var targetUrlStr string

			// 1. FAST PATH: use local sidecar cache to get route for a service
			targetUrlStr, found = sidecar.getNextEndpointFromLocal(serviceName)

			if !found {
				// 2. SLOW PATH IF CACHE MISS: Route NOT FOUND - gRPC call to get route from control plane, then update local cache
				log.Printf("CACHE MISS: Fetching route from Control Plane for service: %s", serviceName)

				// ADD TIMEOUT to Query Control Plane via gRPC
				ctx, cancel := context.WithTimeout(pr.In.Context(), 2*time.Second)
				defer cancel()

				// Dynamically query Control Plane to get LIVE route for that service
				fetchedAddresses, err := sidecar.getRoutesFromControlPlane(ctx, serviceName)
				if err != nil || len(fetchedAddresses) == 0 {
					log.Printf("PROXY ERROR; failed to get route for service: %s", serviceName)
					return // Triggers ErrorHandler
				}

				// 3. UPDATE CACHE in the background, since it was miss before - no addresses in it; add targetUrlStr slice to it
				sidecar.updateLocalCache(serviceName, fetchedAddresses)
				targetUrlStr = fetchedAddresses[0]
			}

			// 4. actually route the request
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

func (sidecar *Sidecar) getRoutesFromControlPlane(ctx context.Context, serviceName string) ([]string, error) {
	if serviceName == "" {
		serviceName = sidecar.serviceName
	}
	req := &pb.RouteRequest{
		ServiceName: serviceName,
	}

	res, err := sidecar.grpcClient.GetRouting(ctx, req)
	if err != nil {
		return nil, err
	}

	if !res.GetSuccess() {
		return nil, fmt.Errorf("Route not found for service: %s", sidecar.serviceName)
	}

	return res.GetAddresses(), nil
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

	return nil
}
