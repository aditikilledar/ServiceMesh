

# Hi! Thanks for checking my repo out. 

# Setup, Getting Started & Testing Guide

This guide walks you through setting up, running, and testing the Service Mesh locally.

---

## 🛠️ Prerequisites & Installation

- **Go**: Version 1.18 or higher installed on your system.
- **Git**: Installed and configured.

### 1. Clone the Repository

```bash
git clone [https://github.com/aditikilledar/ServiceMesh.git](https://github.com/aditikilledar/ServiceMesh.git)
cd ServiceMesh
```

### 2. Install & Verify Dependencies

Resolve all required Go module dependencies:

```bash
go mod tidy
```

---

## 🏃 Getting Started

To demonstrate dynamic service discovery, real-time gRPC pub/sub streaming, and load balancing, run the main driver script:

```bash
go run .
```

### What Happens Behind the Scenes:

1. **Control Plane Initialization**: Starts the gRPC control plane server on port `:50051`.
2. **Upstream Application Servers**: Spawns 3 backend application instances listening on ports `:8081`, `:8082`, and `:8083`.
3. **Route Stream Open**: The sidecar proxy connects to the Control Plane via gRPC to establish a long-lived route streaming connection (`StreamRoutes`).
4. **Dynamic Registration**: The application instances dynamically register their endpoints (`RegisterService`) one by one with a 1-second delay, triggering real-time cache updates across the stream.
5. **Sidecar Launch**: The sidecar HTTP reverse proxy boots up on port `:8080`.

---

## 🧪 Testing the Service Mesh

Once the application logs indicate that the Sidecar Proxy is listening on `:8080`, open a separate terminal to run your tests.

### 1. Basic Proxy Execution & Response Verification

Send a request through the proxy port (`:8080`):

```bash
curl -i [http://127.0.0.1:8080/](http://127.0.0.1:8080/)
```

**Expected Response Headers & Body:**

```http
HTTP/1.1 200 OK
Mesh-Proxy: true
Content-Type: text/plain; charset=utf-8

Hello World :P 
 from 127.0.0.1:8081
```

---

### 2. Testing Atomic Round-Robin Load Balancing

Send multiple consecutive requests to observe the lock-free atomic load balancer distributing traffic evenly across all 3 backend ports (`:8081`, `:8082`, `:8083`):

```bash
for i in {1..6}; do curl [http://127.0.0.1:8080/](http://127.0.0.1:8080/); echo ""; done
```

**Expected Output:**

```text
Hello World :P 
 from 127.0.0.1:8081
Hello World :P 
 from 127.0.0.1:8082
Hello World :P 
 from 127.0.0.1:8083
Hello World :P 
 from 127.0.0.1:8081
Hello World :P 
 from 127.0.0.1:8082
Hello World :P 
 from 127.0.0.1:8083
```

---

### 3. Testing Target Service Headers

Verify that explicit service routing works using the custom `Target-Service` request header:

```bash
curl -H "Target-Service: user-service" [http://127.0.0.1:8080/info](http://127.0.0.1:8080/info)
```

**Expected Output:**

```text
Hi I am reporting something about myself!!!!!
```

---

### 4. Testing Secondary Endpoints

Send requests to secondary backend endpoints to ensure full HTTP routing path propagation:

```bash
curl -i [http://127.0.0.1:8080/info](http://127.0.0.1:8080/info)
```

**Expected Response:**

```http
HTTP/1.1 200 OK
Mesh-Proxy: true
Content-Type: text/plain; charset=utf-8

Hi I am reporting something about myself!!!!!
```

```
```
