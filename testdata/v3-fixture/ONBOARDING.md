# v3-fixture

Tiny Go fixture for /kg-enrich smoke testing — 4 files, 3 architectural layers + a wiring entry point.

## Architecture

### Entry Point

Application entry point that wires all layers together.

Files:
- `main.go` — Wires together the full dependency graph (Store → Service → Handler) and starts an HTTP server on :8080. Serves as the application entry point.

### HTTP API Layer

HTTP handlers that parse requests and write responses.

Files:
- `handler.go` — Defines the HTTP handler layer that bridges incoming requests to the service tier. Decodes query parameters, delegates to Service.GetUser, and encodes JSON responses.

### Service Layer

Business logic orchestrating domain operations.

Files:
- `service.go` — Implements the business logic layer that mediates between the HTTP handler and the data store. Exposes GetUser as the sole domain operation.

### Storage Layer

Data persistence and retrieval abstractions.

Files:
- `store.go` — Provides the in-memory persistence layer with a pre-seeded user map. Exposes Find for context-aware user lookup by ID.

## Tour

### Step 1 — Data Model (~4 min)

Start at the foundation: the `User` struct is the single domain entity in this service, carrying an `ID` and `Name` as JSON-serialisable fields. The `Store` struct wraps a plain `map[string]*User` as its backing store, keeping persistence logic fully in-process and dependency-free. Understanding these two types first gives you the vocabulary used by every other layer. Before moving on, note that `User` is also the value returned all the way up through the service and handler — there is no DTO translation.

Covers:
- `fixture:v3-fixture/store::user` (`user`) — Data transfer object representing a user with an ID and name, serializable to JSON.
- `fixture:v3-fixture/store::store` (`store`) — In-memory store struct backed by a map of user ID to User pointer.

### Step 2 — Store: Construction and Lookup (~5 min)

`NewStore` is the only constructor for storage, pre-seeding the map with a single hard-coded user (`ID=1, Name=Alice`) so the service is immediately usable without external setup. `Find` performs a direct map lookup keyed by string ID, returning a nil pointer (not an error) when the key is absent — a deliberate design choice worth noting for callers. The function accepts a `context.Context` to satisfy the idiomatic Go interface pattern even though it is unused here. Together these two functions define the full read contract that the service layer depends on.

Covers:
- `fixture:v3-fixture/store::newstore` (`newstore`) — Constructs and seeds a Store with a default user record (ID=1, Name=Alice). Returns a pointer to the initialized Store.
- `fixture:v3-fixture/store::find` (`find`) — Looks up a User by ID in the in-memory map, returning nil error on miss. Context parameter is accepted but unused.

### Step 3 — Service Layer (~5 min)

The `Service` struct holds a pointer to `Store`, establishing dependency injection at construction time via `NewService`. `GetUser` is the single business-logic method; in this fixture it is a thin delegation to `store.Find`, but structurally this is where validation, enrichment, or caching would live in a real service. The service layer decouples the HTTP handler from storage, meaning you can swap the store implementation without touching handler code. Trace the call path: `handler → service.GetUser → store.Find` to see the full read flow.

Covers:
- `fixture:v3-fixture/service::service` (`service`) — Struct holding a pointer to Store; encapsulates all domain operations over user data.
- `fixture:v3-fixture/service::newservice` (`newservice`) — Constructs a Service with an injected Store dependency. Returns a pointer to the new Service.
- `fixture:v3-fixture/service::getuser` (`getuser`) — Fetches a user by ID by delegating to Store.Find with the provided context. Passes through the result and error unchanged.

### Step 4 — HTTP Handler (~6 min)

`Handler` holds a reference to `Service` and implements `http.Handler` via `ServeHTTP`, making it directly passable to `http.ListenAndServe`. `NewHandler` injects the service dependency at construction time, mirroring the pattern used in the service layer. Inside `ServeHTTP`, the user ID is extracted from the query string, the service is called with the request context, and the result is JSON-encoded directly to the response writer. Error handling is minimal but intentional: any error from the service produces a 500 with the error message.

Covers:
- `fixture:v3-fixture/handler::handler` (`handler`) — Struct that holds a reference to Service and satisfies http.Handler. Acts as the entry point for all HTTP requests.
- `fixture:v3-fixture/handler::newhandler` (`newhandler`) — Constructs a Handler by injecting a Service dependency. Returns a pointer to the newly created Handler.
- `fixture:v3-fixture/handler::servehttp` (`servehttp`) — Handles an HTTP request by extracting the id query param, calling Service.GetUser, and writing a JSON-encoded User or an error response. Implements http.Handler.

### Step 5 — Entry Point and Wiring (~4 min)

`main` is the composition root of the entire application, constructing all three layers in bottom-up order: `NewStore` → `NewService(store)` → `NewHandler(service)`. The handler is passed directly to `http.ListenAndServe` on port 8080, and `log.Fatal` ensures any startup error surfaces immediately. This explicit wiring makes the dependency graph visible and testable — replacing any layer in tests requires only constructing a substitute and passing it to the next constructor. The pattern is idiomatic Go: no framework, no container, just function calls.

Covers:
- `fixture:v3-fixture/main::main` (`main`) — Bootstraps the application by constructing Store, Service, and Handler in dependency order, then blocks on http.ListenAndServe. Logs fatal on server error.

