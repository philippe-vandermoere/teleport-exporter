# Changelog

All notable changes to the `teleport-exporter` binary/image are documented in
this file. The Helm chart has its own
[CHANGELOG.md](charts/teleport-exporter/CHANGELOG.md), since it's versioned
and released independently.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-10-29

### Added

- Initial release: a Prometheus exporter for a Teleport cluster's
  inventory (SSH nodes, Kubernetes clusters, databases, applications),
  collected in the background and authenticated with a tbot-renewed
  identity file. Metrics: `up`, `nodes_total`, `node_cluster_info`,
  `kubernetes_clusters_total`, `kubernetes_cluster_info`,
  `databases_total`, `databases_by_protocol_total`,
  `databases_by_type_total`, `database_cluster_info`, `apps_total`,
  `app_cluster_info`, `collect_duration_seconds`, `collect_errors_total`,
  `last_successful_collect_timestamp_seconds`, all prefixed
  `teleport_exporter_`.

[Unreleased]: https://github.com/philippe-vandermoere/teleport-exporter/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/philippe-vandermoere/teleport-exporter/releases/tag/v0.1.0
