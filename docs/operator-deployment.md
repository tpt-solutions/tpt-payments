# TPT Payments Operator Deployment Guide

## Prerequisites

### Required Infrastructure

- Kubernetes 1.25+ cluster
- kubectl configured with cluster access
- Helm 3.10+
- PersistentVolume support in cluster
- Ingress controller (nginx-ingress recommended)

### Optional Dependencies

- External PostgreSQL 14+ (if not using bundled)
- External Redis 7+ (if not using bundled)
- External NATS 2.9+ (if not using bundled)
- cert-manager for TLS certificates
- Prometheus + Grafana for monitoring

---

## Installation

### 1. Add Helm Repository

```bash
helm repo add tpt-payments https://charts.tpt-payments.com
helm repo update
```

### 2. Create Namespace

```bash
kubectl create namespace tpt-payments
```

### 3. Configure Values

Create `values.yaml` with your configuration:

```yaml
# values.yaml
replicaCount: 3

image:
  tag: "1.0.0"

ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: api.yourdomain.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: tpt-payments-tls
      hosts:
        - api.yourdomain.com

resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 200m
    memory: 256Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

# Disable bundled dependencies for production
postgresql:
  enabled: false

redis:
  enabled: false

nats:
  enabled: false

# External database configuration
env:
  DB_HOST: postgres.postgres.svc.cluster.local
  DB_PORT: "5432"
  DB_NAME: tpt_payments
  DB_USER: tpt_payments
  REDIS_HOST: redis.redis.svc.cluster.local
  NATS_URL: nats://nats.nats.svc.cluster.local:4222
  LOG_LEVEL: info
  API_RATE_LIMIT: "1000"

# Secrets (use Kubernetes Secrets in production)
existingSecret: tpt-payments-secrets
```

### 4. Create Secrets

```bash
kubectl create secret generic tpt-payments-secrets \
  --namespace tpt-payments \
  --from-literal=DB_PASSWORD=your-db-password \
  --from-literal=REDIS_PASSWORD=your-redis-password \
  --from-literal=JWT_SECRET=your-jwt-secret-key \
  --from-literal=WEBHOOK_SECRET=your-webhook-secret
```

### 5. Install Chart

```bash
helm install tpt-payments tpt-payments/tpt-payments \
  --namespace tpt-payments \
  --values values.yaml
```

---

## First-Time Setup

### Database Migrations

The chart runs the schema migrations in a Helm hook Job (`templates/migrate-job.yaml`) after
install and before each upgrade. The Job uses the same image as the server, with the command
`tpt-pay-server migrate`. Monitor it with:
```bash
kubectl logs -n tpt-payments job/<fullname>-migrate   # fullname: the release's full name (see kubectl get jobs)
```

Set `migrations.enabled: false` to run migrations yourself. Use the same image:
`tpt-pay-server migrate`, with `DATABASE_URL` set. Migrations are serialised by a database advisory
lock, so running the command twice is safe, and a second run applies nothing.

A database created by the docker compose stack (Atlas migrations) is not migrated by this command.
It refuses such a database, because its tables already exist. Keep migrating that database with
Atlas, or start from an empty database.

### Dashboard Users

No default user is created. Dashboard users are made with the `dashuser` command against the core's
database, and the password is read from standard input:

```bash
echo 'a long passphrase' | dashuser create --merchant-email you@example.com \
  --email owner@example.com --name "Owner"
dashuser enroll-totp --email owner@example.com   # prints the authenticator URI once
```

The core needs `DASHBOARD_SERVICE_TOKEN` (at least 32 characters, shared with the dashboard server) and
`DASHBOARD_TOTP_KEY` (`openssl rand -base64 32`, which encrypts second-factor secrets). Without the
service token the internal sign-in routes do not exist.

### Verify Installation

```bash
# Check pod status
kubectl get pods -n tpt-payments

# Check service health
kubectl port-forward svc/tpt-payments 8080:8080 -n tpt-payments
curl http://localhost:8080/health
```

---

## Values Configuration Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of application replicas | `2` |
| `image.tag` | Application image tag | `latest` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `ingress.enabled` | Enable ingress | `false` |
| `resources.limits.cpu` | CPU limit per pod | `1000m` |
| `resources.limits.memory` | Memory limit per pod | `512Mi` |
| `autoscaling.enabled` | Enable horizontal pod autoscaling | `false` |
| `postgresql.enabled` | Use bundled PostgreSQL | `true` |
| `redis.enabled` | Use bundled Redis | `true` |
| `nats.enabled` | Use bundled NATS | `true` |
| `env.API_RATE_LIMIT` | API requests per minute per key | `100` |
| `env.LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

---

## Upgrade Guide

### 1. Update Helm Repository

```bash
helm repo update
```

### 2. Check Available Versions

```bash
helm search repo tpt-payments --versions
```

### 3. Perform Upgrade

```bash
helm upgrade tpt-payments tpt-payments/tpt-payments \
  --namespace tpt-payments \
  --values values.yaml \
  --version <new-version>
```

### 4. Verify Upgrade

```bash
helm list -n tpt-payments
kubectl rollout status deployment/tpt-payments -n tpt-payments
```

---

## Post-Deployment Steps

1. **Access Admin Dashboard:**
   ```
   https://api.yourdomain.com/dashboard
   ```

2. **Configure Webhooks:**
   Navigate to Settings > Webhooks to configure event delivery

3. **Generate API Keys:**
   Navigate to Developers > API Keys to create publishable and secret keys

4. **Configure KYC Provider:**
   Navigate to Settings > KYC to configure identity verification provider

---

## Troubleshooting

### Common Issues

**Pods not starting:**
- Check database connectivity
- Verify secrets are correctly created
- Check persistent volume claims

**Payment processing failing:**
- Verify card network adapter credentials
- Check webhook delivery status
- Review application logs

**Rate limiting:**
- Adjust `API_RATE_LIMIT` environment variable
- Configure per-merchant rate limits in admin dashboard

### Support

- Documentation: https://docs.tpt-payments.com
- Issues: https://github.com/tpt-solutions/tpt-payments/issues
- Community: https://github.com/tpt-solutions/tpt-payments/discussions
