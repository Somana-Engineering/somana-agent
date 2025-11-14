# Sprinter Agent

A Go-based agent for managing and monitoring sensors with a RESTful API.

## Quick Start for Jetson

### Automated Installation

1. Clone the repository:
```bash
git clone https://github.com/miku-kookie/sprinter-agent.git
cd sprinter-agent
```

2. Run the setup script:
```bash
./setup.sh
```

This script will:
- Install Go 1.21.6 for ARM64
- Install all required dependencies
- Download the OpenAPI specification
- Generate code from the API spec
- Build the application
- Set up systemd service for auto-start
- Configure log rotation
- Create basic configuration

### Manual Installation

If you prefer manual installation:

1. Install Go:
```bash
# Download and install Go for ARM64
wget https://go.dev/dl/go1.21.6.linux-arm64.tar.gz
sudo tar -C /usr/local -xzf go1.21.6.linux-arm64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

2. Install dependencies:
```bash
go mod tidy
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
```

3. Download API spec and generate code:
```bash
mkdir -p api
curl -L -o api/openapi.yaml https://github.com/miku-kookie/somana/releases/download/v1.0.2/openapi.yaml
make generate
```

4. Build the application:
```bash
make build
```

## Usage

### Start the Service
```bash
sudo systemctl start sprinter-agent
```

### Check Status
```bash
sudo systemctl status sprinter-agent
```

### View Logs
```bash
journalctl -u sprinter-agent -f
```

### Manual Run (for testing)
```bash
./bin/somana
```

## Configuration

Edit `config/config.yaml` to configure your sensors:

```yaml
server:
  port: 9000
  host: "0.0.0.0"

database:
  path: "./data/somana.db"

sensors:
  temperature:
    type: "temperature"
    location: "room1"
    bridge: "i2c"
```

## API

The API will be available at:
- **API**: http://localhost:9000
- **Documentation**: http://localhost:9000/swagger/index.html

## Development

### Available Make Targets

- `make build` - Generate code and build the application
- `make clean` - Clean build artifacts and generated files
- `make test` - Run tests
- `make deps` - Install dependencies
- `make generate` - Generate code from OpenAPI spec
- `make run` - Generate, build and run the application

### Project Structure

```
sprinter-agent/
├── cmd/server/          # Main application entry point
├── internal/
│   ├── generated/       # Generated code from OpenAPI spec
│   └── services/        # Business logic services
├── api/                 # OpenAPI specification
├── config/              # Configuration files
├── logs/                # Application logs
└── bin/                 # Built binaries
```

## Troubleshooting

### Common Issues

1. **Go not found**: Make sure Go is installed and in your PATH
2. **Permission denied**: Run setup script as non-root user
3. **Service won't start**: Check logs with `journalctl -u sprinter-agent`
4. **Port already in use**: Change port in config or stop conflicting service

### Logs

- Application logs: `./logs/somana.log`
- System logs: `journalctl -u sprinter-agent`

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request 