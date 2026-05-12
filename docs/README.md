## Installation and Setup

### Download and Install

*# Download the latest release*

`wget https://github.com/yourorg/mangahub/releases/latest/mangahub-cli`

*# Make executable (Linux/macOS)*

`chmod +x mangahub-cli`

*# Move to system path*

`sudo mv mangahub-cli /usr/local/bin/mangahub`

*# Verify installation*

`mangahub version`

### First-Time Setup

*# Initialize configuration*

`mangahub init`


## Go Libraries
### Core Framework:

- `github.com/gin-gonic/gin` – HTTP web framework
- `github.com/golang-jwt/jwt/v4` – JWT authentication
- `github.com/gorilla/websocket` – WebSocket support
- `github.com/go-sql-driver/mysql` – MySQL database driver

## Project Structure
```
mangahub/
├── cmd/
│   ├── api-server/main.go      # HTTP API server
│   ├── tcp-server/main.go      # TCP sync server
│   ├── udp-server/main.go      # UDP notification server
│   └── grpc-server/main.go     # gRPC service server
├── internal/
│   ├── auth/                   # Authentication logic
│   ├── manga/                  # Manga data management
│   ├── user/                   # User management
│   ├── tcp/                    # TCP server implementation
│   ├── udp/                    # UDP server implementation
│   ├── websocket/              # WebSocket chat implementation
│   └── grpc/                   # gRPC service implementation
├── pkg/
│   ├── models/                 # Data structures
│   ├── database/               # Database utilities
│   └── utils/                  # Helper functions
├── proto/                      # Protocol Buffer definitions
├── data/                       # JSON data files
├── docs/                       # Documentation
├── docker-compose.yml          # Development environment
└── README.md                   # Setup instructions
```

### References

- https://api.mangadex.org/docs/redoc.html
- https://docs.anilist.co/guide/introduction