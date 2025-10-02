#!/bin/bash

# Stripe CLI webhook forwarding for local development
# This script forwards Stripe webhooks to your local development server
#
# Prerequisites:
# 1. Install the Stripe CLI: https://stripe.com/docs/stripe-cli
# 2. Run `stripe login` to authenticate
#
# Usage:
#   ./stripe-webhooks.sh

# Configuration
LOCAL_WEBHOOK_URL="http://localhost:3000/api/webhooks/stripe"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting Stripe webhook forwarding...${NC}"
echo -e "${YELLOW}Forwarding webhooks to: ${LOCAL_WEBHOOK_URL}${NC}"
echo ""
echo -e "${YELLOW}Important: Copy the webhook signing secret (whsec_...) and add it to your .env file:${NC}"
echo -e "${YELLOW}NUXT_STRIPE_WEBHOOK_SECRET=whsec_...${NC}"
echo ""

# Forward webhooks to local server
# The signing secret will be displayed in the output
stripe listen --forward-to ${LOCAL_WEBHOOK_URL}

