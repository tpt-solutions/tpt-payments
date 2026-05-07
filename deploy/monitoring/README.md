# TPT Credit Chain Monitoring Stack

This directory contains the Prometheus + Grafana monitoring configuration for the TPT Credit chain.

## Quick Start

```bash
# Start the monitoring stack (Prometheus + Grafana + Node Exporter).
docker compose -f docker-compose.monitoring.yml up -d
```

## Accessing the Dashboards

| Service      | URL                                    | Credentials              |
|--------------|----------------------------------------|--------------------------|
| Grafana      | http://localhost:3001                   | admin / tptcredit2026    |
| Prometheus   | http://localhost:9090                   | (no auth)                |
| Node Exporter| http://localhost:9100                   | (no auth)                |

## Metrics Endpoint

The chain node exposes Prometheus metrics at:

```
http://<node-address>:26660/metrics
```

## Available Metrics

| Metric Name                          | Type    | Description                            |
|--------------------------------------|---------|----------------------------------------|
| `tpt_credit_chain_info`             | Gauge   | Chain metadata (chain_id, version)     |
| `tpt_credit_block_height`           | Gauge   | Current block height                   |
| `tpt_credit_human_count`            | Gauge   | Number of registered verified humans   |
| `tpt_credit_supply_ucredit`         | Gauge   | Total yearly issuance supply           |
| `tpt_credit_total_balance_ucredit`  | Gauge   | Total spendable balance                |
| `tpt_credit_supply_per_human`       | Gauge   | Average supply per human               |
| `tpt_credit_avg_balance_ucredit`    | Gauge   | Average account balance                |
| `tpt_credit_events_total`           | Counter | Total events by type                   |
| `tpt_credit_burn_info`              | Gauge   | Last annual burn event details         |
| `tpt_credit_uptime_seconds`         | Counter | Chain uptime in seconds                |
| `tpt_credit_blocks_per_minute`      | Gauge   | Block production rate (moving average) |

## Grafana Dashboard

The TPT Credit Testnet Dashboard is auto-provisioned at startup. It includes panels for:

- Chain health (block height, uptime, blocks/min)
- Account statistics (human count, balances)
- Supply metrics (total supply, per-human averages)
- Event tracking (registrations, issuances, transfers)
- Burn event details (total burned, redistribution amounts)

## Alerting Configuration

To configure alerting rules, create an `alerts.yml` file and reference it in `prometheus.yml`:

```yaml
groups:
  - name: tpt-credit-alerts
    rules:
      - alert: ChainDown
        expr: tpt_credit_uptime_seconds < 60
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "TPT Credit chain is down"
```

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│ Chain Node  │────▶│  Prometheus  │◀────│  Grafana     │
│ :26660      │     │  :9090       │     │  :3001       │
└─────────────┘     └──────┬───────┘     └──────────────┘
                           │
                    ┌──────┴───────┐
                    │ Node Exporter│
                    │ :9100        │
                    └──────────────┘