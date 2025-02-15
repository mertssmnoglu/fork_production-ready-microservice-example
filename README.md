# Example Microservice

A Go-based microservice with Couchbase integration, metrics, logging and OpenTelemetry support.

## Overview

This service provides REST API endpoints for managing products, utilizing Couchbase as the primary database and implementing distributed tracing with OpenTelemetry.

## Prerequisites

- Go 1.23.4
- Docker and Docker Compose
- Couchbase Server

## Tech Stack

- **Language**: Go
- **Database**: Couchbase
- **Distributed Tracing**: OpenTelemetry, Jaeger
- **Container**: Docker
- **Config**: Viper
- **Logger**: Zap
- **Metrics**: Prometheus
- **Monitoring**: Grafana

## Project Structure 

├── app
│ └── product
│ ├── create_product_handler.go
│ ├── get_product_handler.go
│ └── repository.go
├── infra
│ └── couchbase
│── └── repository.go
├── pkg
│ └── config
│── └── config.go
│ └── log
│── └── log.go
├── Dockerfile
├── docker-compose.yml
└── main.go
```

## Getting Started

1. Clone the repository:

```bash
git clone <repository-url>
```

2. Start the required services using Docker Compose:
```bash
docker-compose up -d
```

3. Run the application:
```bash
go run main.go
```

## API Endpoints

### Products

- `POST /products` - Create a new product
- `GET /products/{id}` - Retrieve a product by ID

## Configuration

Configuration is managed through environment variables and config files and can be set in the `docker-compose.yml` file. Key configuration parameters include:

- Couchbase connection settings
- OpenTelemetry configuration
- Application port

## Development

### Building the Docker Image

```bash
docker build -t product-service .
```

### Running Tests

```bash
go test ./...
```

## Observability

This service implements OpenTelemetry for distributed tracing, allowing you to monitor and debug requests across your distributed system.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

[Add your license information here]