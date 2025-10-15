#!/bin/bash

# Gmail Integration Backend - Enhanced Setup Script
# This script automates GCP infrastructure provisioning and environment setup

set -e

# Colors for output (check if terminal supports colors)
if [ -t 1 ] && command -v tput &> /dev/null && [ $(tput colors) -ge 8 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

# Helper function for colored echo
print_color() {
    printf "%b\n" "$1"
}

# Track what was created/configured
CREATED_RESOURCES=()
EXISTING_RESOURCES=()
FAILED_OPERATIONS=()

echo "============================================"
echo "   Gmail Integration Backend Setup"
echo "============================================"
echo ""

# ============================================
# 1. Check for required tools
# ============================================
echo -e "${BLUE}[1/8] Checking for required tools...${NC}"
echo ""

if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed.${NC}"
    echo "   Please install Go 1.24+ from https://go.dev"
    exit 1
fi
echo -e "${GREEN}✅ Go $(go version | awk '{print $3}')${NC}"

if ! command -v gcloud &> /dev/null; then
    echo -e "${RED}❌ gcloud CLI is not installed.${NC}"
    echo "   Install from: https://cloud.google.com/sdk/docs/install"
    exit 1
fi
echo -e "${GREEN}✅ gcloud CLI installed${NC}"

echo ""

# ============================================
# 2. GCP Authentication Check
# ============================================
echo -e "${BLUE}[2/8] Checking GCP authentication...${NC}"
echo ""

# Check if user is authenticated
if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" &> /dev/null; then
    echo -e "${YELLOW}⚠️  Not authenticated with gcloud${NC}"
    echo "   Please run: gcloud auth login"
    exit 1
fi

ACTIVE_ACCOUNT=$(gcloud auth list --filter=status:ACTIVE --format="value(account)" | head -1)
echo -e "${GREEN}✅ Authenticated as: ${ACTIVE_ACCOUNT}${NC}"

# Check if a project is set
CURRENT_PROJECT=$(gcloud config get-value project 2>/dev/null || echo "")
if [ -z "$CURRENT_PROJECT" ]; then
    echo -e "${YELLOW}⚠️  No GCP project is set${NC}"
    echo "   Please run: gcloud config set project YOUR_PROJECT_ID"
    exit 1
fi

echo -e "${GREEN}✅ Using GCP project: ${CURRENT_PROJECT}${NC}"
echo ""

# ============================================
# 3. Environment Configuration
# ============================================
echo -e "${BLUE}[3/8] Setting up environment configuration...${NC}"
echo ""

# Create .env from template if it doesn't exist
if [ ! -f ".env" ]; then
    echo "Creating .env from template..."
    cp env.example .env
    echo -e "${GREEN}✅ Created .env file${NC}"
    echo -e "${YELLOW}   📝 Please edit .env and fill in your values${NC}"
else
    echo -e "${GREEN}✅ .env file already exists${NC}"
fi

# Validate .env file for placeholder values
if [ -f ".env" ]; then
    echo ""
    echo "Checking .env file for placeholder values..."
    
    PLACEHOLDERS_FOUND=false
    
    if grep -q "your-gcp-project-id" .env; then
        echo -e "${YELLOW}⚠️  Found placeholder: your-gcp-project-id${NC}"
        PLACEHOLDERS_FOUND=true
    fi
    
    if grep -q "your-tenant.kinde.com" .env; then
        echo -e "${YELLOW}⚠️  Found placeholder: your-tenant.kinde.com${NC}"
        PLACEHOLDERS_FOUND=true
    fi
    
    if grep -q "your-kinde-client-id" .env; then
        echo -e "${YELLOW}⚠️  Found placeholder: your-kinde-client-id${NC}"
        PLACEHOLDERS_FOUND=true
    fi
    
    if grep -q "your-gmail-oauth-client-id" .env; then
        echo -e "${YELLOW}⚠️  Found placeholder: your-gmail-oauth-client-id${NC}"
        PLACEHOLDERS_FOUND=true
    fi
    
    if [ "$PLACEHOLDERS_FOUND" = true ]; then
        echo -e "${YELLOW}   Please update these placeholder values in .env before running the backend${NC}"
    else
        echo -e "${GREEN}✅ No obvious placeholder values found${NC}"
    fi
fi

# Check service account key file
if [ -f "service-account-key.json" ]; then
    echo -e "${GREEN}✅ Service account key file found${NC}"
    
    # Validate it's valid JSON
    if jq empty service-account-key.json 2>/dev/null; then
        echo -e "${GREEN}✅ Service account key is valid JSON${NC}"
    else
        echo -e "${YELLOW}⚠️  Service account key file exists but is not valid JSON${NC}"
    fi
elif [ -f "../service-account-key.json" ]; then
    echo -e "${GREEN}✅ Service account key file found in parent directory${NC}"
else
    echo -e "${YELLOW}⚠️  Service account key file not found${NC}"
    echo "   You'll need to download a service account key from GCP Console"
    echo "   See: https://cloud.google.com/iam/docs/keys-create-delete"
fi

echo ""

# ============================================
# 4. Enable Required GCP APIs
# ============================================
echo -e "${BLUE}[4/8] Enabling required GCP APIs...${NC}"
echo ""

REQUIRED_APIS=(
    "gmail.googleapis.com"
    "bigquery.googleapis.com"
    "datastore.googleapis.com"
    "cloudkms.googleapis.com"
    "pubsub.googleapis.com"
)

for api in "${REQUIRED_APIS[@]}"; do
    echo "Checking ${api}..."
    
    if gcloud services list --enabled --filter="name:$api" --format="value(name)" | grep -q "$api"; then
        echo -e "${GREEN}✅ ${api} already enabled${NC}"
        EXISTING_RESOURCES+=("API: $api")
    else
        echo "Enabling ${api}..."
        if gcloud services enable "$api" --quiet; then
            echo -e "${GREEN}✅ Enabled ${api}${NC}"
            CREATED_RESOURCES+=("API: $api")
        else
            echo -e "${RED}❌ Failed to enable ${api}${NC}"
            FAILED_OPERATIONS+=("Enable API: $api")
        fi
    fi
done

echo ""

# ============================================
# 5. KMS Setup
# ============================================
echo -e "${BLUE}[5/8] Setting up Cloud KMS...${NC}"
echo ""

KMS_LOCATION="global"
KMS_KEYRING="email-service-keyring"

echo "Checking for KMS keyring: ${KMS_KEYRING}..."

if gcloud kms keyrings describe "$KMS_KEYRING" --location="$KMS_LOCATION" &>/dev/null; then
    echo -e "${GREEN}✅ KMS keyring already exists: ${KMS_KEYRING}${NC}"
    EXISTING_RESOURCES+=("KMS Keyring: $KMS_KEYRING")
else
    echo "Creating KMS keyring: ${KMS_KEYRING}..."
    if gcloud kms keyrings create "$KMS_KEYRING" --location="$KMS_LOCATION" --quiet; then
        echo -e "${GREEN}✅ Created KMS keyring: ${KMS_KEYRING}${NC}"
        CREATED_RESOURCES+=("KMS Keyring: $KMS_KEYRING")
    else
        echo -e "${RED}❌ Failed to create KMS keyring${NC}"
        FAILED_OPERATIONS+=("Create KMS keyring: $KMS_KEYRING")
    fi
fi

echo -e "${BLUE}ℹ️  Per-tenant encryption keys will be created automatically when accounts connect${NC}"
echo ""

# ============================================
# 6. Pub/Sub Setup
# ============================================
echo -e "${BLUE}[6/8] Setting up Pub/Sub...${NC}"
echo ""

PUBSUB_TOPIC="gmail-notifications"

echo "Checking for Pub/Sub topic: ${PUBSUB_TOPIC}..."

if gcloud pubsub topics describe "$PUBSUB_TOPIC" &>/dev/null; then
    echo -e "${GREEN}✅ Pub/Sub topic already exists: ${PUBSUB_TOPIC}${NC}"
    EXISTING_RESOURCES+=("Pub/Sub Topic: $PUBSUB_TOPIC")
else
    echo "Creating Pub/Sub topic: ${PUBSUB_TOPIC}..."
    if gcloud pubsub topics create "$PUBSUB_TOPIC" --quiet; then
        echo -e "${GREEN}✅ Created Pub/Sub topic: ${PUBSUB_TOPIC}${NC}"
        CREATED_RESOURCES+=("Pub/Sub Topic: $PUBSUB_TOPIC")
    else
        echo -e "${RED}❌ Failed to create Pub/Sub topic${NC}"
        FAILED_OPERATIONS+=("Create Pub/Sub topic: $PUBSUB_TOPIC")
    fi
fi

# Create push subscription only if topic creation succeeded
if gcloud pubsub topics describe "$PUBSUB_TOPIC" &>/dev/null; then
    echo ""
    echo "Setting up Pub/Sub push subscription..."
    echo ""
    echo -e "${YELLOW}For local development, you'll need ngrok to receive webhooks.${NC}"
    echo -e "${YELLOW}To set up ngrok:${NC}"
    echo "  1. Install ngrok: https://ngrok.com/download"
    echo "  2. Run: ngrok http 8085"
    echo "  3. Copy the HTTPS URL (e.g., https://abc123.ngrok.io)"
    echo ""

    read -p "Enter your webhook URL (or press Enter to skip): " WEBHOOK_URL

    if [ -n "$WEBHOOK_URL" ]; then
        # Ensure URL ends with the webhook path
        if [[ ! "$WEBHOOK_URL" =~ /webhooks/gmail$ ]]; then
            WEBHOOK_URL="${WEBHOOK_URL}/webhooks/gmail"
        fi
        
        SUBSCRIPTION_NAME="gmail-notifications-sub"
        
        echo "Checking for push subscription: ${SUBSCRIPTION_NAME}..."
        
        if gcloud pubsub subscriptions describe "$SUBSCRIPTION_NAME" &>/dev/null; then
            echo -e "${YELLOW}⚠️  Subscription already exists. Updating...${NC}"
            if gcloud pubsub subscriptions update "$SUBSCRIPTION_NAME" \
                --push-endpoint="$WEBHOOK_URL" \
                --quiet; then
                echo -e "${GREEN}✅ Updated push subscription: ${SUBSCRIPTION_NAME}${NC}"
                EXISTING_RESOURCES+=("Pub/Sub Subscription: $SUBSCRIPTION_NAME (updated)")
            else
                echo -e "${RED}❌ Failed to update subscription${NC}"
                FAILED_OPERATIONS+=("Update subscription: $SUBSCRIPTION_NAME")
            fi
        else
            echo "Creating push subscription: ${SUBSCRIPTION_NAME}..."
            if gcloud pubsub subscriptions create "$SUBSCRIPTION_NAME" \
                --topic="$PUBSUB_TOPIC" \
                --push-endpoint="$WEBHOOK_URL" \
                --quiet; then
                echo -e "${GREEN}✅ Created push subscription: ${SUBSCRIPTION_NAME}${NC}"
                CREATED_RESOURCES+=("Pub/Sub Subscription: $SUBSCRIPTION_NAME")
            else
                echo -e "${RED}❌ Failed to create subscription${NC}"
                FAILED_OPERATIONS+=("Create subscription: $SUBSCRIPTION_NAME")
            fi
        fi
    else
        echo -e "${YELLOW}⚠️  Skipped push subscription setup${NC}"
        echo "   You can create it later with:"
        echo "   gcloud pubsub subscriptions create gmail-notifications-sub \\"
        echo "     --topic=gmail-notifications \\"
        echo "     --push-endpoint=YOUR_WEBHOOK_URL/webhooks/gmail"
    fi
else
    echo ""
    echo -e "${YELLOW}⚠️  Skipping push subscription setup (topic creation failed)${NC}"
fi

echo ""

# ============================================
# 7. Install Backend Dependencies
# ============================================
echo -e "${BLUE}[7/8] Installing backend dependencies...${NC}"
echo ""

echo "Running go mod tidy..."
if go mod tidy; then
    echo -e "${GREEN}✅ Backend dependencies installed${NC}"
else
    echo -e "${RED}❌ Failed to install dependencies${NC}"
    FAILED_OPERATIONS+=("Install Go dependencies")
fi

echo ""

# ============================================
# 8. Setup Summary
# ============================================
echo -e "${BLUE}[8/8] Setup Summary${NC}"
echo ""
echo "============================================"

if [ ${#CREATED_RESOURCES[@]} -gt 0 ]; then
    echo -e "${GREEN}✅ Resources Created:${NC}"
    for resource in "${CREATED_RESOURCES[@]}"; do
        echo "   • $resource"
    done
    echo ""
fi

if [ ${#EXISTING_RESOURCES[@]} -gt 0 ]; then
    echo -e "${BLUE}ℹ️  Existing Resources (not modified):${NC}"
    for resource in "${EXISTING_RESOURCES[@]}"; do
        echo "   • $resource"
    done
    echo ""
fi

if [ ${#FAILED_OPERATIONS[@]} -gt 0 ]; then
    echo -e "${RED}❌ Failed Operations:${NC}"
    for operation in "${FAILED_OPERATIONS[@]}"; do
        echo "   • $operation"
    done
    echo ""
fi

echo "============================================"
echo -e "${GREEN}Setup Complete!${NC}"
echo "============================================"
echo ""

# Next steps
echo -e "${BLUE}📋 Next Steps:${NC}"
echo ""
echo -e "1. ${YELLOW}Configure Credentials:${NC}"
echo "   • Edit .env with your GCP, Kinde, and Gmail credentials"
echo "   • Ensure your service account key file is in place"
echo ""
echo -e "2. ${YELLOW}Get Gmail OAuth Credentials:${NC}"
echo "   • Go to: https://console.cloud.google.com/apis/credentials"
echo "   • Create OAuth 2.0 Client ID (Web application)"
echo "   • Set redirect URI: http://localhost:8085/auth/gmail/callback"
echo "   • Add credentials to .env (GMAIL_CLIENT_ID, GMAIL_CLIENT_SECRET)"
echo ""
echo -e "3. ${YELLOW}Get Kinde Credentials:${NC}"
echo "   • Go to: https://app.kinde.com (or your Kinde domain)"
echo "   • Create/use an application"
echo "   • Add credentials to .env (KINDE_DOMAIN, KINDE_CLIENT_ID, KINDE_CLIENT_SECRET)"
echo "   • Ensure users are assigned to an organization"
echo ""
echo -e "4. ${YELLOW}Start the Backend:${NC}"
echo "   cd backend"
echo "   go run cmd/server/main.go"
echo ""
echo -e "5. ${YELLOW}Set up ngrok for webhooks (if not done):${NC}"
echo "   ngrok http 8085"
echo "   Then update Pub/Sub subscription with the ngrok HTTPS URL"
echo ""
echo -e "6. ${YELLOW}Start the Frontend:${NC}"
echo "   cd ../frontend"
echo "   npm run dev"
echo ""
echo -e "${GREEN}📚 For detailed instructions, see README.md${NC}"
echo ""
