# observability-federation-proxy

A Helm chart for the Observability Federation Proxy - enables centralized Grafana to query Loki/Mimir across multiple Kubernetes clusters

## Installation

```bash
helm install observability-federation-proxy oci://ghcr.io/tjorri/charts/observability-federation-proxy
```

## Cluster Authentication

The proxy supports two authentication methods for connecting to remote clusters:

### EKS Clusters (Recommended for AWS)

For EKS clusters, use IAM Roles for Service Accounts (IRSA) or EKS Pod Identity. No secrets are needed in the Helm chart.

1. Create an IAM role with appropriate permissions
2. Associate the role with the ServiceAccount:

```yaml
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/ObsFederationProxy

clusters:
  - name: prod-eu
    type: eks
    eks:
      clusterName: production-eu-west-1
      region: eu-west-1
    loki:
      namespace: monitoring
      service: loki-gateway
      port: 80
      pathPrefix: /loki
```

### Kubeconfig Clusters

For non-EKS clusters (on-premises, GKE, AKS, etc.), use kubeconfig-based authentication.

> **Security Note**: Kubeconfig files contain sensitive credentials. Follow the practices below to handle them securely.

#### Option 1: External Secret (Recommended for Production)

Create the secret externally using your preferred secret management solution (External Secrets Operator, Sealed Secrets, Vault, etc.):

```yaml
# Created by External Secrets Operator, Terraform, or manually
apiVersion: v1
kind: Secret
metadata:
  name: my-kubeconfigs
type: Opaque
stringData:
  on-prem: |
    apiVersion: v1
    kind: Config
    clusters:
      - cluster:
          server: https://kubernetes.example.com:6443
          certificate-authority-data: <base64>
        name: on-prem
    contexts:
      - context:
          cluster: on-prem
          user: federation-proxy
        name: on-prem
    current-context: on-prem
    users:
      - name: federation-proxy
        user:
          token: <service-account-token>
```

Reference it in your values:

```yaml
clusterSecrets:
  existingSecret: my-kubeconfigs

clusters:
  - name: on-prem
    type: kubeconfig
    kubeconfig:
      path: /etc/kubeconfigs/on-prem  # Key name from the secret
    loki:
      namespace: observability
      service: loki
      port: 3100
      pathPrefix: /loki
```

#### Option 2: Chart-Managed Secret (Development/Testing Only)

For development or testing, the chart can create the secret:

```yaml
clusterSecrets:
  create: true
  kubeconfigs:
    on-prem: |
      apiVersion: v1
      kind: Config
      # ... kubeconfig content

clusters:
  - name: on-prem
    type: kubeconfig
    kubeconfig:
      path: /etc/kubeconfigs/on-prem
    # ...
```

> **Warning**: Avoid passing kubeconfig content via `--set` or unencrypted values files, as credentials may be logged in CI/CD systems or stored in Helm release history.

## Security Best Practices

1. **Use IRSA/Pod Identity for EKS**: No credentials to manage
2. **Use External Secrets for kubeconfigs**: Sync from Vault, AWS Secrets Manager, etc.
3. **Encrypt values files**: Use helm-secrets, SOPS, or similar tools
4. **Limit ServiceAccount permissions**: Create dedicated ServiceAccounts in target clusters with minimal RBAC
5. **Rotate credentials regularly**: Update secrets and restart pods

## Limitations

| Limitation | Description |
|------------|-------------|
| **No dynamic credential rotation** | Changing kubeconfigs requires updating the Secret and restarting pods |
| **Static mount** | Kubeconfigs are mounted at pod startup; changes require pod restart |
| **Single secret** | All kubeconfigs are stored in one Secret when using `clusterSecrets.create` |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules for pod assignment |
| auth.bearerTokens | list | `[]` | Bearer tokens for authentication (use existingSecret for production) |
| auth.enabled | bool | `false` | Enable bearer token authentication |
| clusterSecrets.create | bool | `false` | Create a secret containing kubeconfig files for non-EKS clusters |
| clusterSecrets.existingSecret | string | `""` | Reference to an existing secret containing kubeconfig files (alternative to create) |
| clusterSecrets.kubeconfigs | object | `{}` | Kubeconfig contents keyed by name. Each key becomes a file in /etc/kubeconfigs/. For production, use existingSecret with External Secrets Operator or sealed-secrets instead. |
| clusters | list | `[]` | Cluster configurations. Each cluster can have Loki and/or Mimir endpoints configured. For kubeconfig clusters, reference the kubeconfig by path from clusterSecrets. |
| fullnameOverride | string | `""` | Override the full name of the chart |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"ghcr.io/tjorri/observability-federation-proxy"` | Image repository |
| image.tag | string | `""` | Overrides the image tag whose default is the chart appVersion |
| imagePullSecrets | list | `[]` | Image pull secrets for private registries |
| logging.format | string | `"json"` | Log format (json, text) |
| logging.level | string | `"info"` | Log level (debug, info, warn, error) |
| nameOverride | string | `""` | Override the name of the chart |
| nodeSelector | object | `{}` | Node selector for pod assignment |
| podAnnotations | object | `{}` | Annotations to add to the pod |
| podSecurityContext | object | `{"fsGroup":65534,"runAsNonRoot":true,"runAsUser":65534}` | Pod security context |
| probes.liveness.enabled | bool | `true` | Enable liveness probe |
| probes.liveness.failureThreshold | int | `3` | Number of failures before pod is restarted |
| probes.liveness.initialDelaySeconds | int | `5` | Initial delay before liveness probe starts |
| probes.liveness.periodSeconds | int | `10` | Period between liveness probe checks |
| probes.liveness.timeoutSeconds | int | `5` | Timeout for liveness probe |
| probes.readiness.enabled | bool | `true` | Enable readiness probe |
| probes.readiness.failureThreshold | int | `3` | Number of failures before pod is marked unready |
| probes.readiness.initialDelaySeconds | int | `5` | Initial delay before readiness probe starts |
| probes.readiness.periodSeconds | int | `10` | Period between readiness probe checks |
| probes.readiness.timeoutSeconds | int | `5` | Timeout for readiness probe |
| proxy.listenAddress | string | `":8080"` | Address the proxy listens on |
| proxy.maxTenantHeaderLength | int | `8192` | Maximum length of the X-Scope-OrgID header |
| proxy.metricsEnabled | bool | `true` | Enable Prometheus metrics endpoint |
| proxy.queryTimeout | string | `"30s"` | Timeout for upstream queries |
| replicaCount | int | `1` | Number of replicas for the deployment |
| resources | object | `{"limits":{"cpu":"500m","memory":"256Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Resource limits and requests |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container security context |
| service.port | int | `8080` | Service port |
| service.type | string | `"ClusterIP"` | Service type |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created |
| serviceAccount.name | string | `""` | The name of the service account to use. If not set and create is true, a name is generated using the fullname template |
| serviceMonitor.enabled | bool | `false` | Enable ServiceMonitor for Prometheus Operator |
| serviceMonitor.interval | string | `"30s"` | Scrape interval |
| serviceMonitor.labels | object | `{}` | Additional labels for the ServiceMonitor |
| serviceMonitor.namespace | string | `""` | Namespace for the ServiceMonitor (defaults to release namespace) |
| serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout |
| tolerations | list | `[]` | Tolerations for pod assignment |
