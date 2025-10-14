#!/bin/bash

# Gmail Integration Backend - Setup Script

set -e

echo "==================================="
echo "Gmail Integration Backend Setup"
echo "==================================="
echo ""

# Check for required tools
echo "Checking for required tools..."

if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21+ from https://go.dev"
    exit 1
fi
echo "✅ Go $(go version | awk '{print $3}')"

if ! command -v gcloud &> /dev/null; then
    echo "⚠️  gcloud CLI is not installed. You'll need it for GCP setup."
    echo "   Install from: https://cloud.google.com/sdk/docs/install"
fi

echo ""
echo "==================================="
echo "Environment Configuration"
echo "==================================="
echo ""

# Backend environment
if [ ! -f ".env" ]; then
    echo "Creating .env from template..."
    cp env.example .env
    echo "✅ Created .env"
    echo "   Please edit .env and fill in your values"
else
    echo "✅ .env already exists"
fi

echo ""
echo "==================================="
echo "Installing Dependencies"
echo "==================================="
echo ""

echo "Installing backend dependencies..."
go mod tidy
echo "✅ Backend dependencies installed"

echo ""
echo "==================================="
echo "Next Steps"
echo "==================================="
echo ""
echo "1. Complete GCP Setup:"
echo "   - Create KMS keyring: gcloud kms keyrings create email-service-keyring --location=global"
echo "   - Create Pub/Sub topic: gcloud pubsub topics create gmail-notifications"
echo "   - Enable required APIs (see README.md)"
echo ""
echo "2. Configure Credentials:"
echo "   - Edit .env with your GCP, Kinde, and Gmail credentials"
echo "   - Place your service account key file in this directory"
echo ""
echo "3. Start the backend server:"
echo "   go run cmd/server/main.go"
echo ""
echo "4. Set up ngrok for webhooks (in a separate terminal):"
echo "   ngrok http 8085"
echo ""
echo "5. Configure Pub/Sub subscription with your ngrok URL"
echo ""
echo "See README.md for detailed instructions."
echo ""

