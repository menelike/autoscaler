# Autoscaler (Photologue Fork)

Forked from [woodpecker-ci/autoscaler](https://github.com/woodpecker-ci/autoscaler). Scale your Woodpecker agents automatically based on the current load.

## Fork Changes

### 1. Label-aware capacity counting

**Problem**: The upstream autoscaler counts all connected workers — including non-pool agents like the homelab agent — as available capacity when deciding whether to scale up. With `FILTER_LABELS=type=cloud`, it correctly filters pending/running tasks by label, but still uses the global worker count for capacity. This means the homelab agent inflates available capacity, preventing the autoscaler from provisioning cloud agents for `type=cloud` jobs.

**Fix**: In `engine/autoscaler.go`, `getQueueInfo()` returns `0` instead of `queueInfo.Stats.Workers` for `freeTasks` when `FilterLabels` is set. This way only pool-managed agents count as capacity.

### 2. Billing-aware teardown

**Problem**: Cloud providers like Hetzner bill per hour. The upstream autoscaler tears down idle agents after `AGENT_IDLE_TIMEOUT` (e.g. 5 min), wasting the remaining ~55 minutes you've already paid for.

**Fix**: New `WOODPECKER_BILLING_INTERVAL` and `WOODPECKER_BILLING_BUFFER` options. When set, idle agents stay **schedulable** (not drained) until the billing buffer before the next billing boundary. This maximizes paid-for capacity — the agent can pick up new jobs without reprovisioning.

**Example**: With `BILLING_INTERVAL=1h` and `BILLING_BUFFER=5m`:
- Agent finishes job at minute 20 → stays schedulable until minute 55
- New job at minute 35 → picked up immediately (no cold start)
- No new jobs by minute 55 → agent drained and removed before next billing cycle

## Configuration

### New Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WOODPECKER_BILLING_INTERVAL` | _(disabled)_ | Provider billing cycle (e.g. `1h`). Agents are kept schedulable within the cycle. |
| `WOODPECKER_BILLING_BUFFER` | `5m` | Time before the billing boundary to start teardown. Only used when `BILLING_INTERVAL` is set. |

### All Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WOODPECKER_SERVER` | `http://localhost:8000` | Woodpecker server address |
| `WOODPECKER_TOKEN` | | Woodpecker API token |
| `WOODPECKER_MIN_AGENTS` | `1` | Minimum agents |
| `WOODPECKER_MAX_AGENTS` | `10` | Maximum agents |
| `WOODPECKER_WORKFLOWS_PER_AGENT` | `2` | Max parallel workflows per agent |
| `WOODPECKER_GRPC_ADDR` | `woodpecker-server:9000` | gRPC address for agents |
| `WOODPECKER_GRPC_SECURE` | `false` | Use TLS for gRPC |
| `WOODPECKER_AGENT_IMAGE` | `woodpeckerci/woodpecker-agent:next` | Agent Docker image |
| `WOODPECKER_AGENT_ENV` | | Extra agent environment variables |
| `WOODPECKER_AGENT_IDLE_TIMEOUT` | `10m` | Idle time before agent can be drained |
| `WOODPECKER_AGENT_INACTIVITY_TIMEOUT` | `10m` | Time without server contact before removal |
| `WOODPECKER_FILTER_LABELS` | | Filter tasks by label (e.g. `type=cloud`) |
| `WOODPECKER_BILLING_INTERVAL` | | Provider billing cycle (e.g. `1h`) |
| `WOODPECKER_BILLING_BUFFER` | `5m` | Buffer before billing boundary for teardown |
| `WOODPECKER_PROVIDER` | | Cloud provider (`hetznercloud`, `aws`, `vultr`, `scaleway`) |
| `WOODPECKER_RECONCILIATION_INTERVAL` | `1m` | How often to check and adjust agent pool |

## Building

```bash
docker build -f docker/Dockerfile -t <ecr-registry>/photolog/woodpecker-autoscaler:latest .
docker push <ecr-registry>/photolog/woodpecker-autoscaler:latest
```

## Syncing with upstream

```bash
git remote add upstream https://github.com/woodpecker-ci/autoscaler.git
git fetch upstream
git merge upstream/main
# Resolve conflicts, then rebuild and push
```
