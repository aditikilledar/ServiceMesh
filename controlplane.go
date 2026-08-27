package main

import (
	"context"
	"fmt"
	"log"
	"net"

	pb "servicemesh/proto"

	"google.golang.org/grpc"
)

// NewControlPlane initializes ControlPlane with an empty, non-nil map
func NewControlPlane() *ControlPlane {
	return &ControlPlane{
		routes:      make(map[string][]string), // Allocates empty map
		subscribers: make(map[string][]chan []string),
	}
}

func (ctr *ControlPlane) getRouteMapping(serviceName string) []string {
	ctr.mu.RLock()         // allow concurrent reads
	defer ctr.mu.RUnlock() // release lock while exiting the function

	return ctr.routes[serviceName]
}

// -------------------------------------------------------------------------
// V2 CHANGED: registerRoute appends the new instance address and broadcasts
// the updated endpoint list to all active streaming subscriber channels.
// -------------------------------------------------------------------------
func (ctr *ControlPlane) registerRoute(serviceName string, address string) {
	// Writing data to channels that interact with I/O or external goroutines while holding a mutex is a major anti-pattern.
	// which is why we don't use defer unlock here, what if the channel has some issue and stops, then the lock is still held for an extended period of time.
	ctr.mu.Lock()

	// update new address to route list
	ctr.routes[serviceName] = append(ctr.routes[serviceName], address)

	// let all subscribers know abot this, make a copy so that we can stop holding mutex
	// WRONG: THEY SHARE SAME POINTER updatedList := ctr.routes[serviceName]
	// Adding ... unpacks the slice into individual string elements.
	updatedList := append([]string{}, ctr.routes[serviceName]...)

	// make a list of subscribers
	subChans := append([]chan []string{}, ctr.subscribers[serviceName]...)

	//unlock it now that all work related to control plane access is done
	ctr.mu.Unlock()

	// BROADCAST updatedList to all subscriber channels
	for _, ch := range subChans {
		ch <- updatedList
	}

}

// -------------------------------------------------------------------------
// V2: StreamRoutes implements a gRPC streaming endpoint that establishes a permanent, open connection to a sidecar proxy, instantly feeding it its initial route table and then sitting in an infinite loop waiting to push updates the moment backend services scale or fail.
// -------------------------------------------------------------------------

func (cpServer *ControlPlaneServer) StreamRoutes(req *pb.RouteRequest, stream pb.ControlPlane_StreamRoutesServer) error {
	serviceName := req.GetServiceName()
	// make a buffer of string arrays w/ addresses
	// The sender will only block if the buffer fills up completely
	subscriberChannels := make(chan []string, 10)

	// 1. Register subscriber channel under this service name
	// Lock the Mutex: Prevents other threads from changing routes while setting up this subscriber.
	cpServer.cp.mu.Lock()
	// doesn't append the data in the channels, but adds the empty channel itself
	cpServer.cp.subscribers[serviceName] = append(cpServer.cp.subscribers[serviceName], subscriberChannels)

	// 2. Fetch initial routes snapshot to send to subscriber (sidecar), can't use getRouting here since we already hold the lock
	initialRoutes := append([]string{}, cpServer.cp.routes[serviceName]...)
	cpServer.cp.mu.Unlock()

	// send initial routes of the serviceName to sidecars connected
	if len(initialRoutes) > 0 {
		err := stream.Send(&pb.RouteResponse{
			ServiceName: serviceName,
			Addresses:   initialRoutes,
			Success:     true,
		})
		if err != nil {
			return err
		}
	}

	// 3. Keep stream infinitely open and push any route updates as they arrive on the subscriberChannels
	// code sleeps until a new route is registerd inside the subscriberChannels
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err() // whehn sidecar disconnects

		case addrs := <-subscriberChannels: // triggerred when a new service registers itself
			err := stream.Send(&pb.RouteResponse{
				ServiceName: serviceName,
				Addresses:   addrs,
				Success:     true,
			})
			if err != nil {
				return err
			}
		}
	}

}

// gRPC to allow service registration, fetching serviceName from routes

// Service Registration gRPC
func (cpServer *ControlPlaneServer) RegisterService(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.GetServiceName() == "" || req.GetAddress() == "" {
		return &pb.RegisterResponse{
			Message: "Service Name or Address field cannot be empty.",
			Success: false,
		}, nil
	}

	cpServer.cp.registerRoute(req.GetServiceName(), req.GetAddress())

	return &pb.RegisterResponse{
		Message: "Route registered successfully for given serviceName and address",
		Success: true,
	}, nil
}

// Get Route gRPC
func (cpServer *ControlPlaneServer) GetRouting(ctx context.Context, req *pb.RouteRequest) (*pb.RouteResponse, error) {
	if req.GetServiceName() != "" {
		addresses := cpServer.cp.getRouteMapping(req.GetServiceName())
		log.Print("Found address for service", req.GetServiceName(), ":", addresses)
		if len(addresses) > 0 {
			return &pb.RouteResponse{
				Addresses: addresses,
				Success:   true,
			}, nil
		}
	}

	return &pb.RouteResponse{
		Addresses: []string{},
		Success:   false,
	}, nil
}

func StartControlPlaneServer(port string, cp *ControlPlane) error {
	listener, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", port, err)
	}
	grpcServer := grpc.NewServer()
	server := &ControlPlaneServer{
		cp: cp,
	}

	// this means we agree to implement the interfaces defined in proto files in this handler
	pb.RegisterControlPlaneServer(grpcServer, server)

	log.Printf("Control Plane gRPC server running on %s", port)

	if err := grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("gRPC Server failed to serve: %w", err)
	}

	return nil
}
