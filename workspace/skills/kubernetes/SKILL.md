---
name: kubernetes
description: "Manage Kubernetes clusters with kubectl. Pods, deployments, services, ingress, configmaps, secrets, RBAC, debugging, and scaling."
metadata: {"nanobot":{"emoji":"☸️","requires":{"bins":["kubectl"]},"install":[{"id":"brew","kind":"brew","formula":"kubectl","bins":["kubectl"],"label":"Install kubectl (brew)"}]}}
---

# Kubernetes Skill

Use `kubectl` to manage Kubernetes clusters. Always check current context before running commands.

## Context & Config

```bash
# Show current context
kubectl config current-context

# List all contexts
kubectl config get-contexts

# Switch context
kubectl config use-context production

# Set default namespace
kubectl config set-context --current --namespace=my-app

# View kubeconfig
kubectl config view --minify
```

## Pods

```bash
# List pods
kubectl get pods
kubectl get pods -n kube-system
kubectl get pods -A                   # all namespaces
kubectl get pods -o wide              # show node, IP
kubectl get pods -l app=web           # filter by label
kubectl get pods --field-selector=status.phase=Running

# Describe a pod (events, conditions)
kubectl describe pod my-pod

# Get pod logs
kubectl logs my-pod
kubectl logs my-pod -f                # follow
kubectl logs my-pod --tail=100        # last 100 lines
kubectl logs my-pod --since=10m       # last 10 minutes
kubectl logs my-pod -c sidecar        # specific container
kubectl logs -l app=web --all-containers  # all pods with label

# Exec into a pod
kubectl exec -it my-pod -- bash
kubectl exec -it my-pod -- sh
kubectl exec my-pod -- cat /etc/config/app.yaml

# Port forward
kubectl port-forward pod/my-pod 8080:80
kubectl port-forward svc/my-service 8080:80

# Copy files
kubectl cp my-pod:/var/log/app.log ./app.log
kubectl cp ./config.yaml my-pod:/app/config.yaml

# Delete a pod (it will restart if managed by a deployment)
kubectl delete pod my-pod

# Run a one-off pod
kubectl run debug --image=ubuntu:22.04 --rm -it -- bash
kubectl run curl-test --image=curlimages/curl --rm -it -- curl http://my-service:8080/health
```

## Deployments

```bash
# List deployments
kubectl get deployments
kubectl get deploy -o wide

# Create a deployment
kubectl create deployment web --image=nginx:1.25 --replicas=3

# Scale
kubectl scale deployment web --replicas=5

# Update image (rolling update)
kubectl set image deployment/web web=nginx:1.26

# Rollout status
kubectl rollout status deployment/web

# Rollout history
kubectl rollout history deployment/web

# Rollback
kubectl rollout undo deployment/web
kubectl rollout undo deployment/web --to-revision=3

# Restart (rolling restart without config change)
kubectl rollout restart deployment/web

# Pause/resume rollout
kubectl rollout pause deployment/web
kubectl rollout resume deployment/web
```

## Services

```bash
# List services
kubectl get svc

# Expose a deployment
kubectl expose deployment web --port=80 --target-port=8080 --type=ClusterIP
kubectl expose deployment web --port=80 --target-port=8080 --type=LoadBalancer

# Describe service (endpoints)
kubectl describe svc web

# Get endpoint IPs
kubectl get endpoints web
```

## ConfigMaps & Secrets

```bash
# Create configmap
kubectl create configmap app-config --from-file=config.yaml
kubectl create configmap app-config --from-literal=DB_HOST=10.0.0.5

# Create secret
kubectl create secret generic db-creds --from-literal=password=s3cret!

# View configmap
kubectl get configmap app-config -o yaml

# View secret (base64-decoded)
kubectl get secret db-creds -o jsonpath='{.data.password}' | base64 -d

# Edit in place
kubectl edit configmap app-config
```

## Namespaces

```bash
# List namespaces
kubectl get ns

# Create namespace
kubectl create ns staging

# Delete namespace (deletes ALL resources inside)
kubectl delete ns staging

# Get all resources in a namespace
kubectl get all -n my-app
```

## Apply & Delete (Declarative)

```bash
# Apply manifests
kubectl apply -f deployment.yaml
kubectl apply -f k8s/                   # all files in directory
kubectl apply -f https://raw.githubusercontent.com/...

# Dry run (validate without applying)
kubectl apply -f deployment.yaml --dry-run=server

# Delete resources from manifest
kubectl delete -f deployment.yaml

# Diff (preview changes)
kubectl diff -f deployment.yaml
```

## Resource Management

```bash
# Get all resource types
kubectl api-resources

# Get resource usage
kubectl top nodes
kubectl top pods
kubectl top pods --containers

# Describe a node
kubectl describe node my-node

# Cordon/uncordon (prevent/allow scheduling)
kubectl cordon my-node
kubectl uncordon my-node

# Drain a node (evict pods)
kubectl drain my-node --ignore-daemonsets --delete-emptydir-data
```

## Ingress

```bash
# List ingress
kubectl get ingress

# Describe ingress
kubectl describe ingress my-ingress
```

## RBAC

```bash
# Check if you can do something
kubectl auth can-i create pods
kubectl auth can-i delete deployments --namespace=production

# List cluster roles
kubectl get clusterroles
kubectl get clusterrolebindings

# List role bindings in a namespace
kubectl get rolebindings -n my-app
```

## HPA (Autoscaling)

```bash
# Create autoscaler
kubectl autoscale deployment web --min=2 --max=10 --cpu-percent=70

# List HPAs
kubectl get hpa

# Describe HPA (see current metrics)
kubectl describe hpa web
```

## Debugging

```bash
# Events (sorted by time)
kubectl get events --sort-by='.lastTimestamp'
kubectl get events -n my-app --field-selector reason=Failed

# Debug a CrashLoopBackOff
kubectl describe pod my-pod     # check events section
kubectl logs my-pod --previous  # logs from previous crash

# Debug DNS
kubectl run dns-test --image=busybox --rm -it -- nslookup my-service

# Debug networking
kubectl run curl-test --image=curlimages/curl --rm -it -- curl -v http://my-service:8080

# Node-level debugging
kubectl debug node/my-node -it --image=ubuntu
```

## Common YAML Patterns

### Deployment:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
  selector:
    matchLabels: { app: web }
  template:
    metadata:
      labels: { app: web }
    spec:
      containers:
        - name: web
          image: myapp:v2.1.0
          ports: [{ containerPort: 8080 }]
          resources:
            requests: { cpu: 100m, memory: 128Mi }
            limits: { cpu: 500m, memory: 512Mi }
          readinessProbe:
            httpGet: { path: /health, port: 8080 }
            initialDelaySeconds: 5
          livenessProbe:
            httpGet: { path: /health, port: 8080 }
            initialDelaySeconds: 10
```

## Tips

- Use `-o yaml` to export any resource as YAML: `kubectl get deploy web -o yaml > deploy.yaml`.
- Use `kubectl explain deployment.spec.strategy` to read API docs inline.
- Use `--watch` flag to observe changes: `kubectl get pods --watch`.
- Use `kubectl neat` (plugin) to clean exported YAML of cluster-specific fields.
- Use `kubectx` and `kubens` for fast context/namespace switching.
- Always set resource requests and limits in production.
- Use `--dry-run=client -o yaml` to generate YAML: `kubectl create deploy web --image=nginx --dry-run=client -o yaml`.
