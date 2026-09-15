
# 🛡️ Service Mesh Data Plane (Sidecar Proxy)

The Data Plane component provides an HTTP reverse-proxy sidecar that handles service discovery, dynamic routing, local caching, and load balancing for inter-service communication.

---

## 🛠️ Technical Capabilities

- **Dynamic HTTP Reverse Proxy (`httputil.ReverseProxy`)**Intercepts incoming HTTP requests, extracts target service names from headers (`Target-Service`), rewrites the destination URL to an available instance, and appends mesh telemetry headers (`Mesh-Proxy: true`).
- **Fast-Path In-Memory Route Caching**Stores target backend endpoint lists in an internal thread-safe map (`endpoints map[string][]string`) guarded by `sync.RWMutex`, allowing routing decisions to be resolved locally without network calls to the Control Plane.
- **Lock-Free Atomic Round-Robin Load Balancing**Uses atomic integer increments (`atomic.AddUint64`) with modulo arithmetic across cached endpoint slices to balance incoming request traffic evenly across service instances with sub-microsecond overhead.
- **gRPC Route Streaming (Push Updates)**Establishes a persistent, background gRPC server-stream (`StreamRoutes`) with the Control Plane to receive real-time endpoint updates and hot-reload local routing caches instantly.
- **Control Plane Fallback (Slow Path on Cache Miss)**If a requested target service is missing from local memory, the proxy performs a synchronous gRPC call (`GetRouting`) to the Control Plane with a 2-second context timeout, populating the local cache on success.
- **Service Registration Client**
  Communicates with the Control Plane (`RegisterService`) during initialization to announce new upstream service instances.


# 🎛️ Service Mesh Control Plane

The Control Plane acts as the central brain of the service mesh. It manages service discovery, maintains active endpoint registries, handles service registrations, and streams real-time routing updates to Data Plane sidecars over gRPC.

---

## 🛠️ Technical Capabilities

- **Centralized Service Registry**Maintains an in-memory route map (`routes map[string][]string`) guarded by a `sync.RWMutex` to allow safe concurrent reads and thread-safe dynamic writes as backends scale up or down.
- **Dynamic Service Registration (`RegisterService`)**Exposes a gRPC unary endpoint where backend service instances register their network addresses (`IP:Port`) during startup. Validates input parameters before modifying the central registry.
- **Synchronous Route Lookup (`GetRouting`)**Provides a unary gRPC endpoint allowing sidecars to query the current instance addresses for a given target service on-demand (used during cache misses).
- **Real-Time Pub/Sub Push Updates (`StreamRoutes`)**Implements a long-lived gRPC server-streaming endpoint. Upon connection, it sends an initial route snapshot to the sidecar and then keeps the stream open in a `select` loop, pushing new address lists instantly whenever the service topology changes.
- **Lock-Free Broadcast Pattern**When a new service registers, `registerRoute` copies the active slice of subscribers (`subscribers map[string][]chan []string`) and releases the mutex *before* writing to buffered channels (`chan []string`). This prevents channel I/O bottlenecks from blocking main registry state operations.
- **gRPC Network Server (`StartControlPlaneServer`)**Initializes a standard TCP listener and registers the generated Protobuf interfaces (`pb.RegisterControlPlaneServer`) to serve incoming Data Plane requests.

# ⚙️ Application Service & Business Logic

The Application Service file (`businesslogic.go`) represents the upstream target backend microservices that run behind the Data Plane sidecar proxies in the mesh ecosystem.

---

## 🛠️ Technical Capabilities

- **Isolated HTTP Multiplexing (`http.NewServeMux`)**Creates localized, instance-specific request routers rather than attaching routes to Go's default global HTTP multiplexer. This guarantees complete isolation when running multiple app instances concurrently within the same process.
- **Dynamic Handler Injection**Supports optional variadic handler parameterization (`handler ...http.Handler`). If a custom handler or proxy middleware is provided during startup, the server uses it; otherwise, it falls back to its own internal router.
- **Business Endpoint Handlers**

  - `/` (`Home`): Emits instance metadata logs and responds with a plain-text payload identifying the active server port.
  - `/info` (`Info`): Provides diagnostic and status reporting for the running backend service instance.
- **Non-Blocking Concurrent Lifecycle Management**Integrates with `sync.WaitGroup` to coordinate synchronized startup routines across multiple goroutines, signaling completion prior to executing the blocking `http.ListenAndServe` call.
- **Address & URL Self-Identification (`URL()`)**
  Exposes helper utilities to construct fully-qualified HTTP address strings (`http://<port>`), enabling the service to announce its endpoint location to the Control Plane upon registration.
