#!/bin/bash

# Exit on any error
set -e

# Change to project root if script is run from scripts/ directory
cd "$(dirname "$0")/.."

echo "--------------------------------------------------------"
echo "🚀 Initializing Pulse Hybrid Deployment"
echo "--------------------------------------------------------"

# 1. Start infrastructure (Databases, Redis, etc.)
echo "📥 Starting persistent infrastructure via Docker Compose..."
docker compose up -d

# 2. Wait for some time
echo "⏳ Waiting for databases to initialize (10s)..."
sleep 10

# 3. Build the images
echo "🔨 Building API and Worker images..."
docker build -f Dockerfile.api -t pulse-api:latest .
docker build -f Dockerfile.worker -t pulse-worker:latest .

# 4. Load images into minikube
echo "📦 Transferring images to Minikube..."
minikube image load pulse-api:latest
minikube image load pulse-worker:latest

# 5. Apply K8s manifests in order
echo "☸️ Orchestrating Kubernetes resources..."
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/secrets.yaml
kubectl apply -f deploy/k8s/pulse-api.yaml
kubectl apply -f deploy/k8s/pulse-worker.yaml

# 6. Force a rollout to ensure new images are picked up (since we reuse :latest)
echo "♻️ Restarting deployments to apply image updates..."
kubectl -n pulse rollout restart deployment pulse-api pulse-worker

echo ""
echo "--------------------------------------------------------"
echo "✅ Deployment sequence finished!"
echo "--------------------------------------------------------"
echo "Current status in 'pulse' namespace:"
kubectl -n pulse get pods
