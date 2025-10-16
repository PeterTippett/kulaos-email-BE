#!/bin/bash

# GCP Secret Manager Setup Script
# This script creates secrets in GCP Secret Manager for sensitive configuration values

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ID="${PROJECT_ID:-}"

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

# Function to check prerequisites
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

# Function to enable Secret Manager API
enable_secret_manager_api() {
    print_status "Enabling Secret Manager API..."
    
    gcloud services enable secretmanager.googleapis.com --quiet
    
    print_success "Secret Manager API enabled"
}

# Function to create a secret with its initial value
create_secret() {
    local secret_name="$1"
    local secret_description="$2"
    local secret_value="$3"
    
    print_status "Creating secret: $secret_name"
    
    # Check if secret already exists
    if gcloud secrets describe "$secret_name" &> /dev/null; then
        print_warning "Secret $secret_name already exists"
        return
    fi
    
    # Create the secret with its initial value
    echo -n "$secret_value" | gcloud secrets create "$secret_name" \
        --data-file=- \
        --replication-policy="automatic" \
        --labels="app=email-service,environment=production" \
        --quiet
    
    print_success "Secret $secret_name created with initial value"
}

# Function to add secret version
add_secret_version() {
    local secret_name="$1"
    local secret_value="$2"
    
    print_status "Adding version to secret: $secret_name"
    
    # Add the secret value
    echo -n "$secret_value" | gcloud secrets versions add "$secret_name" --data-file=-
    
    print_success "Secret version added for $secret_name"
}

# Function to grant App Engine access to secrets
grant_secret_access() {
    print_status "Granting App Engine access to secrets..."
    
    local app_engine_sa="$PROJECT_ID@appspot.gserviceaccount.com"
    
    # Check if App Engine service account exists
    if ! gcloud iam service-accounts describe "$app_engine_sa" &> /dev/null; then
        print_warning "App Engine service account does not exist yet."
        print_status "This is normal if App Engine app hasn't been created yet."
        print_status "IAM permissions will be granted during deployment."
        return
    fi
    
    # Grant access to all secrets
    local secrets=("kinde-client-id" "kinde-client-secret" "gmail-client-id" "gmail-client-secret")
    
    for secret in "${secrets[@]}"; do
        print_status "Granting access to secret: $secret"
        
        gcloud secrets add-iam-policy-binding "$secret" \
            --member="serviceAccount:$app_engine_sa" \
            --role="roles/secretmanager.secretAccessor" \
            --quiet
    done
    
    print_success "App Engine access granted to all secrets"
}

# Function to prompt for secret values
prompt_for_secrets() {
    echo ""
    print_status "Please provide the following secret values:"
    echo ""
    
    # Kinde secrets
    read -p "Kinde Client ID: " kinde_client_id
    read -s -p "Kinde Client Secret: " kinde_client_secret
    echo ""
    
    # Gmail secrets
    read -p "Gmail Client ID: " gmail_client_id
    read -s -p "Gmail Client Secret: " gmail_client_secret
    echo ""
    
    # Store values in variables
    KINDE_CLIENT_ID="$kinde_client_id"
    KINDE_CLIENT_SECRET="$kinde_client_secret"
    GMAIL_CLIENT_ID="$gmail_client_id"
    GMAIL_CLIENT_SECRET="$gmail_client_secret"
}

# Function to create all secrets
create_all_secrets() {
    print_status "Creating all secrets..."
    
    # Create secrets with their initial values
    create_secret "kinde-client-id" "Kinde OAuth Client ID" "$KINDE_CLIENT_ID"
    create_secret "kinde-client-secret" "Kinde OAuth Client Secret" "$KINDE_CLIENT_SECRET"
    create_secret "gmail-client-id" "Gmail OAuth Client ID" "$GMAIL_CLIENT_ID"
    create_secret "gmail-client-secret" "Gmail OAuth Client Secret" "$GMAIL_CLIENT_SECRET"
    
    print_success "All secrets created and configured"
}

# Function to show next steps
show_next_steps() {
    echo ""
    print_status "Next steps:"
    echo "1. Update your app.yaml with actual non-sensitive values:"
    echo "   - PROJECT_ID: $PROJECT_ID"
    echo "   - KINDE_DOMAIN: your-actual-kinde-domain.kinde.com"
    echo "   - FRONTEND_BASE_URL: https://your-actual-frontend-domain.com"
    echo ""
    echo "2. Deploy your application (this will also grant Secret Manager access):"
    echo "   ./deploy.sh"
    echo ""
    echo "3. Update OAuth redirect URIs after deployment:"
    echo "   - Kinde: https://your-app-id.appspot.com/api/auth/kinde/callback"
    echo "   - Gmail: https://your-app-id.appspot.com/auth/gmail/callback"
    echo ""
    print_status "Useful commands:"
    echo "  List secrets: gcloud secrets list"
    echo "  View secret: gcloud secrets versions access latest --secret=SECRET_NAME"
    echo "  Update secret: echo 'new-value' | gcloud secrets versions add SECRET_NAME --data-file=-"
}

# Function to verify secrets
verify_secrets() {
    print_status "Verifying secrets..."
    
    local secrets=("kinde-client-id" "kinde-client-secret" "gmail-client-id" "gmail-client-secret")
    
    for secret in "${secrets[@]}"; do
        if gcloud secrets describe "$secret" &> /dev/null; then
            print_success "Secret $secret exists"
        else
            print_error "Secret $secret does not exist"
            exit 1
        fi
    done
    
    print_success "All secrets verified"
}

# Main execution
main() {
    echo "=========================================="
    echo "GCP Secret Manager Setup Script"
    echo "=========================================="
    echo ""
    
    check_prerequisites
    enable_secret_manager_api
    prompt_for_secrets
    create_all_secrets
    grant_secret_access
    verify_secrets
    show_next_steps
    
    echo ""
    print_success "Secret Manager setup completed successfully!"
}

# Run main function
main "$@"
