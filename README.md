# 🕸️ Custom Service Mesh MVP (Go + gRPC)

A lightweight, custom implementation of a **Service Mesh** built in Go. This MVP separates the **Control Plane** (dynamic service discovery, registration, and gRPC route streaming) from the **Data Plane** (sidecar reverse proxies with in-memory caching and lock-free atomic load balancing) to manage traffic between clients and backend microservices.

Check out a detailed overview of each of the building blocks [here](BUILDINGBLOCKS.md).

---

## 🚀 Key Features & Capabilities

* **Separation of Control and Data Planes**: Clear decoupling between service discovery orchestration and network proxy execution.
* **Sidecar Reverse Proxy**: Intercepts HTTP traffic, rewrites routing destinations using `httputil.ProxyRequest`, and injects custom telemetry headers (`Mesh-Proxy: true`).
* **Fast-Path In-Memory Route Caching**: Caches endpoint address lists directly inside sidecar memory (`endpoints map[string][]string`) using `sync.RWMutex` to eliminate per-request Control Plane network hops.
* **Atomic Round-Robin Load Balancing**: Alternates traffic across healthy backend instances using lock-free atomic operations (`atomic.AddUint64`) and modulo arithmetic for sub-microsecond routing execution.
* **Real-Time Push Updates via gRPC Streams**: Sidecars open long-lived server streams (`StreamRoutes`) to automatically receive pushed endpoint updates in real-time as backends register.
* **Fallback Cache-Miss Handling**: Automatically falls back to a synchronous gRPC route lookup (`GetRouting`) with a 2-second timeout when an un-cached service is requested.

---

## 🧱 Component Breakdown

```
                                  +-----------------------------+
                                  |    Control Plane (gRPC)     |
                                  |           :50051            |
                                  +--------------+--------------+
                                                 |
                       gRPC Registration         |  gRPC Route Streaming
                       & Route Lookups           |  (Push Updates)
                                                 v
+----------------+        +---------------+------+       +-------------------+
|                |  HTTP  |               | HTTP |------>| App Instance 1    |
|  Client /      |------->| Data Plane    |      |       | 127.0.0.1:8081    |
|  Consumer      |        | Sidecar Proxy |      |       +-------------------+
|                |        | :8080         | HTTP |------>| App Instance 2    |
+----------------+        |               |      |       | 127.0.0.1:8082    |
                          | (Round-Robin) |      |       +-------------------+
                          |               | HTTP |------>| App Instance 3    |
                          +---------------+      |       | 127.0.0.1:8083    |
                                                 +------>+-------------------+
```

* **`controlplane.go`**: Implements the gRPC server (`RegisterControlPlaneServer`). Manages the central in-memory route registry, handles dynamic service registrations, and broadcasts updates to streaming subscriber channels without holding locks during channel I/O.
* **`dataplane.go`**: Defines the `Sidecar` proxy structure, managing local endpoint caches, atomic load balancing counters, and background stream listeners to update cached routes dynamically.
* **`businesslogic.go`**: Contains the upstream target `ApplicationServer` running on its own isolated multiplexer (`http.NewServeMux`), simulating backend microservices (`/` and `/info` endpoints).
* **`main.go`**: The central driver program that orchestrates the execution lifecycle across all mesh components in a synchronized boot sequence.

---

## 🎬 Lifecycle & Boot Sequence (`main.go`)

The driver script demonstrates end-to-end initialization in 7 steps:

1. **Start Control Plane Server**: Spawns the Control Plane gRPC server on TCP port `:50051`.
2. **Port Readiness Check**: Executes `waitForPort()` to block until the Control Plane gRPC port is active and ready to accept connections.
3. **Start Application Backend**: Initializes and boots an `ApplicationServer` on `127.0.0.1:8081` using a `sync.WaitGroup` to confirm startup.
4. **Initialize Sidecar Proxy**: Constructs a new `Sidecar` proxy instance for `user-service` configured to listen on HTTP port `:8080`.
5. **Register Service Endpoint**: Sends a unary gRPC request (`RegisterService`) to register `[http://127.0.0.1:8081](http://127.0.0.1:8081)` under `user-service` in the Control Plane registry.
6. **Establish Route Stream**: Calls `StartRouteStream()` to open a long-lived gRPC server stream with the Control Plane, immediately ingesting the registered initial snapshot into the sidecar's local cache.
7. **Start Sidecar Listener**: Starts the sidecar HTTP proxy server on `:8080` to begin intercepting and routing requests.

---

## 📋 Prerequisites

* **Go**: Version 1.18 or higher installed on your machine.
* **Protocol Buffers**: Generated Go stubs (`servicemesh/proto`) compiled using `protoc` and `protoc-gen-go-grpc`.

---

## 🛠️ Getting Started & Setup

### 1. Clone the Repository

```bash
git clone https://github.com/aditikilledar/ServiceMesh.git
cd ServiceMesh
```

### 2. Synchronize Dependencies

Ensure module dependencies are resolved:

```bash
go mod tidy
```

### 3. Run the Main Mesh Driver

Start the full orchestrator simulation:

```bash
go run .
```

### 4. Test the Proxy

In a separate terminal window, issue an HTTP request through the sidecar proxy on port `:8080`:

```bash
curl -i http://127.0.0.1:8080/
```

**Response Output:**

```http
HTTP/1.1 200 OK
Mesh-Proxy: true
Content-Type: text/plain; charset=utf-8

Hello World :P 
 from 127.0.0.1:8081
```
