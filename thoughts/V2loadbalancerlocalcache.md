### Features Implemented So Far

**Data Plane (Sidecar Proxy)**

* **Dynamic Reverse Proxying:** Configured Go’s `httputil.ReverseProxy` with custom `Rewrite` logic to route incoming HTTP traffic dynamically.
* **Control Plane Integration:** Intercepts incoming HTTP requests to query the Control Plane via gRPC for live service resolution (`GetRouting`).
* **Header & Context Manipulation:** Automatically injects tracing headers (`Mesh-Proxy: true`) and handles dynamic service overrides via `Target-Service` request headers.
* **Service Registration:** Automatically registers local sidecar endpoints with the Control Plane at startup (`RegisterServiceWithControlPlane`).
* **Resilience Mechanisms:** Built-in context deadlines (`context.WithTimeout`) on outbound gRPC calls to prevent hanging requests.

**Control Plane (Service Registry & Routing Server)**

* **Service Registry:** Maintains an in-memory mapping of service names to network addresses (`serviceName` **$\rightarrow$** `address`).
* **gRPC Server Implementation:** Exposes high-performance gRPC endpoints (`RegisterService` and `GetRouting`) defined via Protocol Buffers.
* **Thread-Safe Memory Management:** Concurrency protection using `sync.RWMutex` (`Lock` for writes, `RLock` for reads) to handle concurrent sidecar requests safely.

## WE ARE ADDING

* **In-Memory Endpoint Caching (Data Plane):** The sidecar stores the list of backend service URLs directly in its own memory. This lets the proxy pick a route instantly without making a network call to the Control Plane for every incoming HTTP request.
* **Local Round-Robin Load Balancing:** Instead of relying on the Control Plane to choose an address, the sidecar uses atomic counter operations (`atomic.AddUint64`) to alternate between healthy backend instances locally in microsecond speeds.
* **Real-Time Push Updates via gRPC Streaming:** The sidecar opens a long-lived gRPC server stream (`StreamRoutes`). Whenever a new service instance registers, the Control Plane pushes the updated address list down the open stream so the sidecar can update its local cache immediately in the background.
