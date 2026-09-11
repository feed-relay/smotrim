# Feed Relay Provider for Smotrim

This project is a provider for the [feed-relay system](https://github.com/feed-relay/feed-relay) that converts Smotrim shows into Apple Podcasts-compatible RSS feeds.

## Architecture Overview

The provider follows a simple pipeline to transform Smotrim data into an RSS feed:

1. **Provider**: The main orchestrator. It manages concurrency, handles subscriptions, and coordinates the fetching of data and the construction of the feed.
2. **Client**: An API client that interacts with Smotrim's backend. It uses:
    - **GraphQL API**: To retrieve brand metadata, channel information, and episode lists.
    - **Player API**: To retrieve specific audio stream links and durations for each episode.
3. **Adapter**: A transformation layer that maps Smotrim's domain models (Brands, Episodes, Audio) to the RSS format using the `rsscast` library.
4. **FileSizer**: A utility to resolve the actual byte size of remote media files, which is required for high-quality RSS feeds.
5. **XmlFileWriter**: The final stage of the pipeline, which persists the generated RSS feed to the local filesystem.

### Subscription Management

To simplify managing a large number of shows, the provider includes a `SubsUpdater` service. This service:
- Fetches all available brands from Smotrim.
- Filters brands to include only those that are published, public, and categorized as `Radiobroadcast`.
- Groups the filtered brands by their respective channels.
- Generates a `etc/subscriptions.smotrim.yml` file, which is used by the main provider to automatically configure subscriptions., which persists the generated RSS feed to the local filesystem.

The pipeline includes built-in feed validation to ensure the output is compliant with the RSS specification and gracefully handles missing audio streams to maintain feed stability.

## Development Guide

### Prerequisites

- Go 1.21+
- `golangci-lint` (for linting)
- `moq` (for generating mocks)

### Building and Running

You can run the example application to see the provider in action:

```bash
go run cmd/example/main.go
```

By default, it uses `etc/config.yml`. You can specify a different config file or use production data:

```bash
go run cmd/example/main.go -config path/to/config.yml -prod
```

You can also specify the XML output directory using the `-out` flag:

```bash
go run cmd/example/main.go -out /path/to/output
```

#### Updating Subscriptions

To automatically update the list of available subscriptions, run the `update-subs` utility:

```bash
go run cmd/update-subs/main.go
```

Available flags:
- `-config`: Path to the YAML config file (defaults to `etc/config.yml`).
- `-prod`: Use production data instead of test data.

### Configuration

The application is configured via a YAML file. Key configuration fields include:

- `output_dir`: The directory where generated RSS files will be saved.
- `itunes_owner_name`: The name of the feed owner, used in the Apple Podcasts metadata.
- `itunes_owner_email`: The email of the feed owner, used in the Apple Podcasts metadata.
- `generator`: A string describing the tool that generated the feed.
- `subscriptions`: A list of subscription configurations. Each subscription consists of:
    - `limit`: The maximum number of episodes to include in the feed for this subscription.
    - `shows`: A list of Smotrim show IDs to include in the feed.

### Development Commands

The project uses a `Makefile` to simplify common tasks:

- **Code Generation**: Run `make generate` to update mocks.
- **Formatting**: Run `make fmt` to format the source code.
- **Linting**: Run `make lint` to check for code smells and errors.
- **Quick Check**: Run `make check` to perform formatting, vet, and linting checks before committing.

## Testing

Testing is integrated into the Makefile:

- **Standard Tests**: Run `make test` to run all tests with race detection and output coverage.
- **Race Detection**: Run `make race` for a focused race detection run with a longer timeout.

## License

This project is licensed under the terms specified in the LICENSE file.
