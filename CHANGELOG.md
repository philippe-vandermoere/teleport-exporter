# Changelog

All notable changes to the `teleport-exporter` binary/image are documented in
this file. The Helm chart has its own
[CHANGELOG.md](charts/teleport-exporter/CHANGELOG.md), since it's versioned
and released independently.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-11-02

### Changed

- `teleport_exporter_kubernetes_cluster_info`,
  `teleport_exporter_database_cluster_info` and
  `teleport_exporter_app_cluster_info` gain `node_name` (the serving
  agent's UUID) and `version` (its Teleport version) labels, and now have
  one series per resource **and agent** instead of one per resource: a
  resource served by several agents (HA) gets one series per agent. The
  `*_total` metrics still count each resource once. Queries that assumed a
  single series per resource (e.g. `count(…_cluster_info)`) must aggregate
  by the resource name label instead.

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

[Unreleased]: https://github.com/philippe-vandermoere/teleport-exporter/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/philippe-vandermoere/teleport-exporter/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/philippe-vandermoere/teleport-exporter/releases/tag/v0.1.0
