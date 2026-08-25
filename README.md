<p align="center">
  <img src="https://raw.githubusercontent.com/sig9org/update-radar/main/assets/logo.webp" alt="update-radar">
</p>

# Update Radar

[![Go Reference](https://pkg.go.dev/badge/github.com/sig9org/update-radar.svg)](https://pkg.go.dev/github.com/sig9org/update-radar)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`update-radar` is a Go CLI that monitors Cisco software pages, GitHub
repositories, and arbitrary web pages. It stores observations in a state file
next to the configuration and sends changes through chatxgo (Cisco Webex,
Microsoft Teams, Slack, or Discord).

## Configuration

Copy [`config.yml.example`](config.yml.example) to `config.yml`. Targets are
grouped under `profiles`; the common `settings` block controls timeouts,
concurrency, logging, and browser behavior. Mentions are configured
independently under each notification destination. Credentials and
destination URLs must be supplied through configuration or deployment
secrets; they are not embedded in the program.

The state filename follows the configuration filename: `config.yml` becomes
`config_state.yml`, while `all.yaml` becomes `all_state.yaml`.

## Usage

```text
update-radar -config config.yml
update-radar -config config.yml -dryrun
update-radar -init
update-radar -h
update-radar -v
```

`-dryrun` never sends notifications or writes state. `-silent` suppresses
normal output, while `-debug` takes precedence and enables diagnostic output.

## Embedded use

Other Go tools can import `pkg/radar` and select the categories to check:

```go
result, err := radar.Check(ctx, radar.Options{
    ConfigPath: "config.yml",
    Targets:    []radar.Target{radar.Cisco, radar.GitHub},
    ReadOnly:   true,
})
if err != nil { /* handle the check error */ }
if result.Updated { /* at least one target changed */ }
```

An empty `Targets` slice checks all configured categories. Embedded checks do
not send notifications; they return an aggregate result and persist state by
default.

To retrieve current observations directly, use the public fetch functions:

```go
cisco, err := radar.FetchCisco(ctx, radar.CiscoSite{
    Name: "APIC",
    URL:  "https://software.cisco.com/download/home/285968390/type/286278832/release/",
}, radar.CiscoOptions{Headless: true})
// cisco.Suggested, cisco.Latest, and cisco.Deferred include expanded child releases.

github, err := radar.FetchGitHub(ctx, "https://github.com/owner/repository", radar.GitHubOptions{})
web, err := radar.FetchWeb(ctx, "https://example.com/", radar.WebOptions{})
```

`FetchGitHub` returns the latest release and tag when both checks are enabled
(the default). `FetchWeb` returns the SHA-256 digest of the response body.

## Development

```text
go test ./...
go vet ./...
govulncheck ./...
osv-scanner -r .
```
