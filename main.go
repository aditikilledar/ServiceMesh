package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	pb "servicemesh/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var waitgroup sync.WaitGroup

	cp := NewControlPlane()

	// 1. Start Control Plane gRPC Server
	go func() {
		if err := StartControlPlaneServer(":50051", cp); err != nil {
			log.Fatalf("Control Plane gRPC server failed: %v", err)
		}
	}()

	waitForPort("127.0.0.1:50051", 3*time.Second)

	// 2. Connect gRPC Client
	conn, err := grpc.NewClient("127.0.0.1:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Could not connect to control plane: %v", err)
	}
	defer conn.Close()
	grpcClient := pb.NewControlPlaneClient(conn)

	// 3. Start Backend App Server
	realApplication := &ApplicationServer{
		listenPort: "127.0.0.1:8081",
	}
	waitgroup.Add(1)

	go func() {
		if err := realApplication.StartAppServer(&waitgroup); err != nil {
			log.Fatal("Error starting Application: ", err)
		}
	}()

	waitgroup.Wait()
	log.Printf("Application Server running on %s. Starting Sidecar Proxy...", realApplication.listenPort)

	// 4. Initialize Sidecar
	sidecarProxy := NewSidecar("user-service", ":8080", grpcClient)

	// 5. Register with Control Plane FIRST
	ctxRegistration, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sidecarProxy.RegisterServiceWithControlPlane(ctxRegistration, realApplication.URL()); err != nil {
		log.Fatalf("Failed to register sidecar: %v", err)
	}

	// 6. Start Streaming SECOND (so initial snapshot receives the registered route)
	ctxStream := context.Background()
	sidecarProxy.StartRouteStream(ctxStream, sidecarProxy.serviceName)

	// 7. Start HTTP Proxy (blocking), nil waitgroup because we don't want to wait for anything while starting this server
	if err := sidecarProxy.StartSidecar(nil); err != nil {
		log.Fatal(err)
	}
}

func waitForPort(addr string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
