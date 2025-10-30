#!/bin/bash

# GCP App Engine Deployment Script for Email Service
# This script deploys the Go backend to Google Cloud App Engine

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ID="${PROJECT_ID:-}"
REGION="${REGION:-australia-southeast1}"
SERVICE_NAME="${SERVICE_NAME:-default}"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if gcloud is installed and authenticated
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check if gcloud is installed
    if ! command -v gcloud &> /dev/null; then
        print_error "gcloud CLI is not installed. Please install it first:"
        echo "https://cloud.google.com/sdk/docs/install"
        exit 1
    fi
    
    # Check if user is authenticated
    if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" | grep -q .; then
        print_error "Not authenticated with gcloud. Please run:"
        echo "gcloud auth login"
        exit 1
    fi
    
    # Check if project is set
    if [ -z "$PROJECT_ID" ]; then
        print_error "PROJECT_ID environment variable is not set."
        echo "Please set it: export PROJECT_ID=your-gcp-project-id"
        exit 1
    fi
    
    # Set the project
    gcloud config set project "$PROJECT_ID"
    
    print_success "Prerequisites check passed"
}

# Function to enable required APIs
enable_apis() {
    print_status "Enabling required GCP APIs..."
    
    local apis=(
        "appengine.googleapis.com"
        "datastore.googleapis.com"
        "bigquery.googleapis.com"
        "cloudkms.googleapis.com"
        "pubsub.googleapis.com"
        "gmail.googleapis.com"
        "cloudscheduler.googleapis.com"
        "secretmanager.googleapis.com"
    )
    
    for api in "${apis[@]}"; do
        print_status "Enabling $api..."
        gcloud services enable "$api" --quiet
    done
    
    print_success "All required APIs enabled"
}

# Function to create App Engine application
create_app_engine_app() {
    print_status "Creating App Engine application..."
    
    # Check if App Engine app already exists
    if gcloud app describe &> /dev/null; then
        print_warning "App Engine application already exists"
        return
    fi
    
    # Create App Engine app
    gcloud app create --region="$REGION" --quiet
    
    print_success "App Engine application created"
}

# Function to grant Secret Manager access to App Engine
grant_secret_manager_access() {
    print_status "Granting Secret Manager access to App Engine..."
    
    local app_engine_sa="$PROJECT_ID@appspot.gserviceaccount.com"
    local secrets=("kinde-client-id" "kinde-client-secret" "gmail-client-id" "gmail-client-secret" "twilio-account-sid" "twilio-auth-token" "mcp-shared-key")
    
    for secret in "${secrets[@]}"; do
        print_status "Granting access to secret: $secret"
        
        gcloud secrets add-iam-policy-binding "$secret" \
            --member="serviceAccount:$app_engine_sa" \
            --role="roles/secretmanager.secretAccessor" \
            --quiet
    done
    
    print_success "Secret Manager access granted to App Engine"
}

# Function to validate app.yaml configuration
validate_config() {
    print_status "Validating app.yaml configuration..."
    
    if [ ! -f "app.yaml" ]; then
        print_error "app.yaml file not found in current directory"
        exit 1
    fi
    
    # Check for placeholder values in app.yaml (excluding secrets which are handled by Secret Manager)
    if grep -q "your-gcp-project-id\|your-tenant.kinde.com\|your-app-id\|your-frontend-domain.com" app.yaml; then
        print_error "app.yaml contains placeholder values. Please update them with actual values:"
        grep -n "your-gcp-project-id\|your-tenant.kinde.com\|your-app-id\|your-frontend-domain.com" app.yaml
        exit 1
    fi
    
    # Check if secrets exist in Secret Manager
    local secrets=("kinde-client-id" "kinde-client-secret" "gmail-client-id" "gmail-client-secret" "twilio-account-sid" "twilio-auth-token" "mcp-shared-key")
    for secret in "${secrets[@]}"; do
        if ! gcloud secrets describe "$secret" &> /dev/null; then
            print_error "Secret $secret does not exist in Secret Manager."
            echo "Please run ./setup-secrets.sh first to create the secrets."
            exit 1
        fi
    done
    
    print_success "app.yaml configuration validated"
}

# Function to deploy the application
deploy_app() {
    print_status "Deploying application to App Engine..."
    
    # Remove any existing main.go in root (from previous deployments)
    if [ -f "main.go" ]; then
        print_status "Removing existing main.go from root..."
        rm main.go
    fi
    
    # Create temporary main.go in root for App Engine deployment
    if [ -f "cmd/server/main.go" ]; then
        print_status "Copying main.go from cmd/server/ to root for App Engine deployment..."
        cp cmd/server/main.go main.go
        MAIN_GO_COPIED=true
    else
        print_error "cmd/server/main.go not found!"
        exit 1
    fi
    
    # Deploy the app
    gcloud app deploy app.yaml --quiet --version="v$(date +%Y%m%d-%H%M%S)"
    
    # Clean up temporary main.go if we copied it
    if [ "$MAIN_GO_COPIED" = true ]; then
        print_status "Cleaning up temporary main.go from root..."
        rm main.go
    fi
    
    print_success "Application deployed successfully"
}

# Function to get the deployed URL
get_app_url() {
    local app_url=$(gcloud app browse --no-launch-browser 2>/dev/null || echo "")
    if [ -n "$app_url" ]; then
        print_success "Application is available at: $app_url"
        echo ""
        print_status "Health check: $app_url/health"
        print_status "API endpoints: $app_url/api/*"
    fi
}

# Function to show next steps
show_next_steps() {
    echo ""
    print_status "Next steps:"
    echo "1. Update your Kinde OAuth redirect URI to: https://$(gcloud app describe --format='value(defaultHostname)')/api/auth/kinde/callback"
    echo "2. Update your Gmail OAuth redirect URI to: https://$(gcloud app describe --format='value(defaultHostname)')/auth/gmail/callback"
    echo "3. Update your Twilio webhook URLs in Twilio console:"
    echo "   - https://console.twilio.com/us1/develop/phone-numbers/manage/incoming/PN1929e6a106e2f174cac8f96b24b13099/configure"
    echo "   - Inbound SMS: https://$(gcloud app describe --format='value(defaultHostname)')/webhooks/{org_id}/twilio/sms"
    echo "   - Note: Status callbacks are set automatically when sending SMS"
    echo "4. Update your frontend configuration to use the new backend URL"
    echo "5. Test the health endpoint: https://$(gcloud app describe --format='value(defaultHostname)')/health"
    echo "6. Set up Cloud Scheduler for Gmail watch refresh (see infrastructure/gcloud/setup-cloud-scheduler.sh)"
    echo "7. Make sure pubsub subscription is pointing to server"
    echo "   - https://console.cloud.google.com/cloudpubsub/subscription/edit/gmail-notifications-sub?project=kulaos-email-prod&authuser=1&tab=overview"
    echo "   - Endpoint: https://$(gcloud app describe --format='value(defaultHostname)')/webhooks/{org_id}/twilio/sms"
    echo ""
    print_status "Secret Management:"
    echo "  List secrets: gcloud secrets list"
    echo "  View secret: gcloud secrets versions access latest --secret=SECRET_NAME"
    echo "  Update secret: echo 'new-value' | gcloud secrets versions add SECRET_NAME --data-file=-"
    echo ""
    print_status "Useful commands:"
    echo "  View logs: gcloud app logs tail -s default"
    echo "  View app info: gcloud app describe"
    echo "  Browse app: gcloud app browse"
}

# Main execution
main() {
    echo "=========================================="
    echo "GCP App Engine Deployment Script"
    echo "=========================================="
    echo ""
    
    check_prerequisites
    enable_apis
    create_app_engine_app
    grant_secret_manager_access
    validate_config
    deploy_app
    get_app_url
    show_next_steps
    
    echo ""
    print_success "Deployment completed successfully!"
}

# Run main function
main "$@"
