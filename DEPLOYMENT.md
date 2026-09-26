# Temporary Photolog autoscaler integration

This deployment branch starts from upstream `main` at
`57b3eff2da53f0c6e5f020ce56cceba1447e38af` and combines:

- [PR #581](https://github.com/woodpecker-ci/autoscaler/pull/581), scheduler and
  provider capability support, at `648050cfa419505f56f2108faccf044148763ea9`.
- [PR #751](https://github.com/woodpecker-ci/autoscaler/pull/751), hourly billing
  rollover, at `598c69b58024435191974597fc7b00ce00be682a`, adapted to #581's
  `engine/agents.go`. Reactivation also checks the current bucket so agents
  drained for outdated labels or capabilities stay drained.

The former `feat/label-aware-pool-capacity` patch is not included. #581 supplies
the label and capacity accounting instead. The upstream PR #751 remains a
separate, single commit based on `main`.

## Retirement

**This integration is obsolete once both PRs are merged and an upstream image
containing both changes is available.** Verify that the combined upstream code
preserves the rollover and label matching behavior, then replace the private
image in Photolog's bastion Compose file with that upstream image, pinned by
digest. Remove this deployment override and its rebuild instructions at that
point. A merge alone does not change the running image.

## Validation

The added rollover tests fail without the adapted fix. They cover reuse with
zero, one, or two pending cloud jobs, unrelated homelab demand/global workers,
repeated reconciliation while an agent boots, running work across a billing
boundary, next-window removal, invalid timestamps, label drift, and update
failure preserving the local drained state.

```sh
go test -race -cover -timeout 30s ./...
golangci-lint run
CGO_ENABLED=0 lint go.woodpecker-ci.org/autoscaler/cmd/woodpecker-autoscaler
```

Build from a clean checkout with its `.git` directory present so the binary's
version includes the source commit. Deployment configuration, image digest,
targeted restart commands, and rollback image are recorded in
`photolog/infrastructure/terraform/README.md`. Wait for the CI queue to finish
before restarting the autoscaler.
