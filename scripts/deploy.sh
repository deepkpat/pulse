#!/bin/bash

# Exit on any error
set -e

# Change to project root if script is run from scripts/ directory
cd "$(dirname "$0")/.."

echo "--------------------------------------------------------"
echo "🚀 Initializing Pulse Full Kubernetes Deployment"
echo "--------------------------------------------------------"

# Ensure metrics-server is enabled for HPA
echo "📊 Ensuring metrics-server is enabled in Minikube..."
minikube addons enable metrics-server

# 1. Build the images
echo "🔨 Building API and Worker images..."
docker build -f Dockerfile.api -t pulse-api:latest .
docker build -f Dockerfile.worker -t pulse-worker:latest .

# 2. Load images into minikube
echo "📦 Transferring images to Minikube..."
minikube image load pulse-api:latest
minikube image load pulse-worker:latest

# 3. Setup Namespace and Secrets
echo "☸️ Setting up namespace and secrets..."
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/secrets.yaml

# 4. Deploy Databases (Infrastructure)
echo "📥 Deploying persistent infrastructure..."
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/clickhouse.yaml

# 5. Wait for databases to be ready
echo "⏳ Waiting for databases to initialize..."
kubectl -n pulse wait --for=condition=ready pod -l app=postgres --timeout=120s
kubectl -n pulse wait --for=condition=ready pod -l app=redis --timeout=120s
kubectl -n pulse wait --for=condition=ready pod -l app=clickhouse --timeout=120s

# 6. Deploy Applications
echo "🚀 Deploying Pulse API and Worker..."
kubectl apply -f k8s/pulse-api.yaml
kubectl apply -f k8s/pulse-worker.yaml

# 7. Deploy Monitoring
echo "📈 Deploying Monitoring stack..."
kubectl apply -f k8s/prom.yaml

# 8. Force a rollout to ensure new images are picked up
echo "♻️ Restarting deployments to apply image updates..."
kubectl -n pulse rollout restart deployment pulse-api pulse-worker

echo ""
echo "--------------------------------------------------------"
echo "✅ Deployment sequence finished!"
echo "--------------------------------------------------------"
echo "Current status in 'pulse' namespace:"
kubectl -n pulse get all
