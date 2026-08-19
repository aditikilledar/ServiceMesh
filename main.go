package main

import (
	"context"
	"log"
	pb "servicemesh/proto"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var waitgroup sync.WaitGroup

	cp := NewControlPlane()

	// 2. Start Control Plane gRPC Server in a background goroutine
	go func() {
		if err := StartControlPlaneServer(":50051", cp); err != nil {
			log.Fatalf("Control Plane gRPC server failed: %v", err)
		}
	}()

	// Connect gRPC client to control plane
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Could not connect to control plane: %v", err)
	}
	defer conn.Close()
	grpcClient := pb.NewControlPlaneServiceClient(conn)

	realApplication := &ApplicationServer{
		listenPort: "127.0.0.1:8081",
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
		serviceName: "user-service",
		targetUrl:   "http://localhost:8081",
		proxy:       nil,
		listenPort:  ":8080",
		grpcClient:  grpcClient,
	}
	// Sidecar registers the service with Control Plane
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sidecarProxy.RegisterServiceWithControlPlane(ctx, sidecarProxy.targetUrl); err != nil {
		log.Fatalf("Failed to register sidecar: %v", err)
	}

	sidecarProxy.proxy = sidecarProxy.CreateDynamicProxy()

	// pass nil waitgroup bc we don't wanna wait for anything while starting this server
	err = sidecarProxy.StartSidecar(nil)
	if err != nil {
		log.Fatal(err)
	}
}
