# Changelog

All notable changes to the `teleport-exporter` Helm chart are documented in
this file. The exporter binary/image has its own
[CHANGELOG.md](../../CHANGELOG.md), since it's versioned and released
independently (`appVersion` here tracks which image version the chart
defaults to).

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-10-29

### Added

- Initial release: a Deployment running the exporter alongside a tbot
  native sidecar (diagnostics endpoint enabled, with startup, liveness and
  readiness probes), joining Teleport with the `kubernetes` join method
  (`static_jwks` or `oidc` token type) through a projected ServiceAccount
  token whose audience defaults to the host part of `teleport.address`.
  `identity.existingSecret` as an alternative to tbot, Service, optional
  Prometheus Operator `ServiceMonitor`.

[Unreleased]: https://github.com/philippe-vandermoere/teleport-exporter/compare/chart-v0.1.0...HEAD
[0.1.0]: https://github.com/philippe-vandermoere/teleport-exporter/releases/tag/chart-v0.1.0
