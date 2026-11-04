package diffreview

// DemoPair is a release-review story: a checkout rollout that pins nothing,
// scales to zero, drops probes and limits, mounts the node, exposes a
// LoadBalancer, loosens HPA, deletes a NetworkPolicy, and adds cluster-admin.
func DemoPair() (before, after []Object, err error) {
	before, err = ParseYAML("demo-before", []byte(demoBefore))
	if err != nil {
		return nil, nil, err
	}
	after, err = ParseYAML("demo-after", []byte(demoAfter))
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

const demoBefore = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout
  namespace: shop
spec:
  replicas: 3
  selector:
    matchLabels:
      app: checkout
  template:
    metadata:
      labels:
        app: checkout
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
      containers:
      - name: checkout
        image: ghcr.io/example/checkout:1.4.2
        env:
        - name: API_KEY
          valueFrom:
            secretKeyRef:
              name: checkout-secrets
              key: api
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 256Mi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
        volumeMounts:
        - name: data
          mountPath: /var/lib/checkout
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: checkout-data
---
apiVersion: v1
kind: Service
metadata:
  name: checkout
  namespace: shop
spec:
  type: ClusterIP
  selector:
    app: checkout
  ports:
  - name: http
    port: 80
    targetPort: 8080
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: checkout
  namespace: shop
spec:
  minReplicas: 2
  maxReplicas: 10
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: checkout
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: checkout-net
  namespace: shop
spec:
  podSelector:
    matchLabels:
      app: checkout
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: shop
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: checkout-reader
  namespace: shop
spec:
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: Role
    name: view
  subjects:
  - kind: ServiceAccount
    name: checkout
    namespace: shop
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: edge
  namespace: platform
spec:
  replicas: 2
  selector:
    matchLabels:
      app: edge
  template:
    metadata:
      labels:
        app: edge
    spec:
      containers:
      - name: edge
        image: ghcr.io/example/edge:2.0.0
`

const demoAfter = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout
  namespace: shop
spec:
  replicas: 0
  selector:
    matchLabels:
      app: checkout
  template:
    metadata:
      labels:
        app: checkout
    spec:
      securityContext:
        runAsNonRoot: false
        runAsUser: 0
      containers:
      - name: checkout
        image: ghcr.io/example/checkout:latest
        securityContext:
          privileged: true
          capabilities:
            add:
            - NET_ADMIN
        env:
        - name: API_KEY
          valueFrom:
            secretKeyRef:
              name: checkout-secrets
              key: api-v2
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 200m
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
        volumeMounts:
        - name: data
          mountPath: /var/lib/checkout
        - name: node-logs
          mountPath: /var/log
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: checkout-data
      - name: node-logs
        hostPath:
          path: /var/log
---
apiVersion: v1
kind: Service
metadata:
  name: checkout
  namespace: shop
spec:
  type: LoadBalancer
  selector:
    app: checkout
  ports:
  - name: http
    port: 8080
    targetPort: 9090
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: checkout
  namespace: shop
spec:
  minReplicas: 8
  maxReplicas: 4
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: checkout
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 90
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: checkout-reader
  namespace: shop
spec:
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: Role
    name: view
  subjects:
  - kind: ServiceAccount
    name: checkout-admin
    namespace: shop
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: checkout-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: checkout
  namespace: shop
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: edge
  namespace: platform
spec:
  replicas: 2
  selector:
    matchLabels:
      app: edge
  template:
    metadata:
      labels:
        app: edge
    spec:
      containers:
      - name: edge
        image: ghcr.io/example/edge:2.0.0
`
