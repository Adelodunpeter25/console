# Console Agent - AI Coding Agent Daemon

A persistent daemon service for AI-powered coding assistance with a unified CLI interface.

## Installation

### Local Development
```bash
npm install
npm run cli
```

### Global Installation
```bash
npm link
console --help
```

## CLI Commands

### Start Daemon
```bash
console start
```

Options:
- `-p, --port <number>` - Port to run the server on (default: 3000)
- `-h, --host <string>` - Host to bind to (default: 0.0.0.0)
- `--no-daemon` - Run in foreground instead of background

### Stop Daemon
```bash
console stop
```

### Check Status
```bash
console status
```

### View Logs
```bash
console logs
```

Options:
- `-f, --follow` - Follow log output (tail -f)
- `-n, --lines <number>` - Number of lines to show (default: 50)

### Restart Daemon
```bash
console restart
```

## Architecture

The console agent runs as a persistent daemon service with:

- **PID Management**: Tracks process ID in `~/.console/daemon.pid`
- **Logging**: Structured logs to `~/.console/logs/daemon.log`
- **Configuration**: Settings in `~/.console/config.json`
- **Graceful Shutdown**: Handles SIGTERM/SIGINT properly
- **Health Monitoring**: Process uptime and status tracking

## API Server

The daemon exposes a REST API on `http://0.0.0.0:3000` by default:

- **Base URL**: `http://localhost:3000/api`
- **Health Check**: `GET /api/health`
- **Agent Sessions**: `POST /api/sessions`, `GET /api/sessions`, etc.

## File Structure

```
~/.console/
├── daemon.pid          # Process ID file
├── config.json         # Daemon configuration
└── logs/
    └── daemon.log      # Application logs
```

## Development

Run the server in foreground for development:
```bash
console start --no-daemon
```

Or use the dev script:
```bash
npm run dev
```

## Production Deployment

For server deployment:

1. Install globally: `npm link`
2. Start daemon: `console start`
3. Configure systemd/supervisor for auto-restart
4. Set up reverse proxy (nginx) for SSL
5. Configure firewall rules

## License

MIT