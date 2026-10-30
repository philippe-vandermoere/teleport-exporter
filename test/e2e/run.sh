#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")"

# Files this run needs outside Terraform's state, removed on exit.
WORK_DIR=$(mktemp -d)

# The Teleport provider joins as a bot with the kubernetes join method and
# reads its ServiceAccount token from here; Terraform writes it (see
# teleport.tf).
export KUBERNETES_TOKEN_PATH="$WORK_DIR/terraform-sa-token"
export TF_VAR_kubernetes_token_path="$KUBERNETES_TOKEN_PATH"
export TF_IN_AUTOMATION=1
export KUBECONFIG="$WORK_DIR/kubeconfig"

PORT_FORWARD_PID=""

cleanup() {
    if [ -n "$PORT_FORWARD_PID" ]; then kill "$PORT_FORWARD_PID" 2>/dev/null || true; fi
    if [ "${KEEP:-0}" = "1" ]; then
        echo "--- KEEP=1: leaving the environment up. To use it, then remove it: ---"
        echo "export KUBECONFIG=$KUBECONFIG KUBERNETES_TOKEN_PATH=$KUBERNETES_TOKEN_PATH TF_VAR_kubernetes_token_path=$KUBERNETES_TOKEN_PATH"
        echo "terraform -chdir=$PWD destroy && rm -r $WORK_DIR"
        return
    fi
    echo "--- tearing down ---"
    if ! terraform destroy -auto-approve -input=false -no-color >/dev/null 2>&1; then
        # The Teleport provider joins even to destroy, so a destroy fails
        # whenever Teleport is unreachable (e.g. an apply that failed before
        # it came up): remove the k3d cluster's Docker objects directly.
        echo "terraform destroy failed, removing the k3d cluster directly"
        remove_k3d_cluster
    fi
    rm -rf "$WORK_DIR"
}
trap cleanup EXIT

remove_k3d_cluster() {
    local cluster="${TF_VAR_cluster_name:-teleport-exporter-e2e}"
    docker ps -aq --filter "label=k3d.cluster=$cluster" | xargs -r docker rm -f >/dev/null
    docker network rm "k3d-$cluster" >/dev/null 2>&1 || true
    docker volume ls -q --filter "label=k3d.cluster=$cluster" | xargs -r docker volume rm >/dev/null
    rm -f terraform.tfstate terraform.tfstate.backup
}

fail() {
    echo "FAILED: $1"
    kubectl get pods -A || true
    for target in e2e/deploy/teleport-exporter teleport/deploy/teleport-auth teleport/deploy/teleport-proxy; do
        echo "--- $target logs ---"
        kubectl -n "${target%%/*}" logs "${target#*/}" --all-containers --tail=50 || true
    done
    echo "--- last metrics ---"; metrics | grep '^teleport_' || true
    exit 1
}

# wait_for DESCRIPTION TIMEOUT COMMAND [ARG...] runs COMMAND every second
# until it succeeds, failing the test after TIMEOUT seconds.
wait_for() {
    local description="$1" timeout="$2"
    shift 2
    local waited=0
    echo -n "waiting for: $description "
    while ! "$@"; do
        if [ "$waited" -ge "$timeout" ]; then
            echo "timed out after ${timeout}s"
            fail "$description"
        fi
        sleep 1
        waited=$((waited + 1))
        echo -n "."
    done
    echo "ok (${waited}s)"
}

metrics() { curl -fsS http://localhost:9180/metrics 2>/dev/null || true; }

# metric_value prints the value of the first series matching the given
# exact "name{labels}" prefix (labels in Prometheus' sorted order).
metric_value() { metrics | grep -F "$1 " | head -n1 | awk '{print $NF}'; }

# metric_is NAME_AND_LABELS VALUE succeeds if that series has that value.
metric_is() { [ "$(metric_value "$1")" = "$2" ]; }

# metrics_match REGEX succeeds if any line of /metrics matches it.
metrics_match() { metrics | grep -q "$1"; }

echo "--- terraform apply: k3d, Teleport, 2 SSH nodes, 2 k3s clusters, the exporter ---"
terraform init -input=false -no-color >/dev/null
# The Teleport provider joins the cluster as soon as it is configured (at
# plan time), so Teleport and the provider's join token come first.
terraform apply -auto-approve -input=false -no-color -target=local_sensitive_file.terraform_token
(umask 077 && terraform output -raw kubeconfig > "$KUBECONFIG")
terraform apply -auto-approve -input=false -no-color

kubectl -n e2e port-forward svc/teleport-exporter 9180:9180 >/dev/null 2>&1 &
PORT_FORWARD_PID=$!

wait_for "exporter up" 120 metrics_match "^teleport_exporter_up 1"

c='cluster_name="teleport.teleport.svc.cluster.local"'

wait_for "2 SSH nodes" 60 metric_is "teleport_exporter_nodes_total{$c}" 2
wait_for "2 Kubernetes clusters" 60 metric_is "teleport_exporter_kubernetes_clusters_total{$c}" 2
wait_for "2 apps" 60 metric_is "teleport_exporter_apps_total{$c}" 2
wait_for "2 databases" 60 metric_is "teleport_exporter_databases_total{$c}" 2

echo "--- checking per-resource info metrics ---"
teleport_version=$(terraform output -raw teleport_version)
for node in ssh-node-1 ssh-node-2; do
    metrics | grep -qE "^teleport_exporter_node_cluster_info\{address=\"[^\"]*\",$c,hostname=\"$node\",node_name=\"[0-9a-f-]{36}\",version=\"$teleport_version\"\} 1$" \
        || fail "missing node_cluster_info for $node"
done
for kube in k3s-1 k3s-2; do
    metrics | grep -qF "teleport_exporter_kubernetes_cluster_info{$c,kube_cluster_name=\"$kube\"} 1" \
        || fail "missing kubernetes_cluster_info for $kube"
done
metrics | grep -qF "teleport_exporter_app_cluster_info{app_name=\"grafana\",$c,public_addr=\"grafana.teleport.test\",uri=\"http://grafana.e2e.test:3000\"} 1" \
    || fail "missing app_cluster_info for grafana"
metrics | grep -qF "teleport_exporter_app_cluster_info{app_name=\"prometheus\",$c,public_addr=\"prometheus.teleport.test\",uri=\"http://prometheus.e2e.test:9090\"} 1" \
    || fail "missing app_cluster_info for prometheus"
metrics | grep -qF "teleport_exporter_database_cluster_info{$c,database_name=\"postgres\",protocol=\"postgres\",type=\"self-hosted\"} 1" \
    || fail "missing database_cluster_info for postgres"
metrics | grep -qF "teleport_exporter_database_cluster_info{$c,database_name=\"mysql\",protocol=\"mysql\",type=\"self-hosted\"} 1" \
    || fail "missing database_cluster_info for mysql"
echo "ok"

echo "--- checking database breakdowns ---"
[ "$(metric_value "teleport_exporter_databases_by_protocol_total{$c,protocol=\"postgres\"}")" = "1" ] || fail "databases_by_protocol_total{protocol=postgres} is not 1"
[ "$(metric_value "teleport_exporter_databases_by_protocol_total{$c,protocol=\"mysql\"}")" = "1" ] || fail "databases_by_protocol_total{protocol=mysql} is not 1"
[ "$(metric_value "teleport_exporter_databases_by_type_total{$c,type=\"self-hosted\"}")" = "2" ] || fail "databases_by_type_total{type=self-hosted} is not 2"
echo "ok"

echo "--- checking exporter health metrics ---"
[ "$(metric_value "teleport_exporter_collect_errors_total{$c}")" = "0" ] || fail "collect_errors_total is not 0"
metrics | grep -q "^teleport_exporter_collect_duration_seconds{$c} " || fail "missing collect_duration_seconds"
last_success=$(metric_value "teleport_exporter_last_successful_collect_timestamp_seconds{$c}")
age=$(( $(date +%s) - $(printf '%.0f' "$last_success") ))
[ "$age" -lt 30 ] || fail "last successful collection is ${age}s old"
echo "ok (last successful collection ${age}s ago)"

echo "--- scaling the Teleport auth service down to exercise a collection failure ---"
kubectl -n teleport scale deploy/teleport-auth --replicas=0
wait_for "exporter down" 120 metrics_match "^teleport_exporter_up 0"
errors=$(metric_value "teleport_exporter_collect_errors_total{$c}")
[ "${errors:-0}" -gt 0 ] || fail "collect_errors_total did not increase while Teleport was down"
# The last successful inventory stays published while Teleport is down.
[ "$(metric_value "teleport_exporter_nodes_total{$c}")" = "2" ] || fail "nodes_total was not kept from the last successful collection"
echo "collect_errors_total=$errors -- ok"

echo "--- scaling it back up ---"
kubectl -n teleport scale deploy/teleport-auth --replicas=1
wait_for "exporter up again" 180 metrics_match "^teleport_exporter_up 1"

echo "--- all checks passed ---"
