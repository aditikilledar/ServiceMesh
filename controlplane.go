package main

import (
	"context"
	"fmt"
	"log"
	"net"

	pb "servicemesh/proto"

	"google.golang.org/grpc"
)

func (ctr *ControlPlane) getRouteMapping(serviceName string) string {
	return ctr.routes[serviceName]
}

func (ctr *ControlPlane) registerRoute(serviceName string, address string) {
	ctr.routes[serviceName] = address
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
func (cpServer *ControlPlaneServer) GetRoute(ctx context.Context, req *pb.RouteRequest) (*pb.RouteResponse, error) {
	if req.GetServiceName() != "" {
		address := cpServer.cp.getRouteMapping(req.GetServiceName())
		if address != "" {
			return &pb.RouteResponse{
				Address: address,
				Success: true,
			}, nil
		}
	}

	return &pb.RouteResponse{
		Address: "",
		Success: false,
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

	pb.RegisterControlPlaneServiceServer(grpcServer, server)

	log.Printf("Control Plane gRPC server running on %s", port)

	if err := grpcServer.Serve(listener); err != nil {
		fmt.Errorf("gRPC Server failed to serve: %w", err)
	}

	return nil
}

// NewControlPlane initializes ControlPlane with an empty, non-nil map
func NewControlPlane() *ControlPlane {
	return &ControlPlane{
		routes: make(map[string]string), // Allocates empty map
	}
}
