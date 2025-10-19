#!/bin/bash
# Generate ED25519 SSH key pair for Zeitwork reconciler

KEY_NAME=${1:-"zeitwork-reconciler-key"}
OUTPUT_DIR=${2:-"."}

echo "Generating SSH key pair for Zeitwork..."
echo "Key name: $KEY_NAME"

# Generate ED25519 key pair
ssh-keygen -t ed25519 -f "$OUTPUT_DIR/$KEY_NAME" -N "" -C "$KEY_NAME"

# Extract public key
PUB_KEY=$(cat "$OUTPUT_DIR/$KEY_NAME.pub")

# Read private key (base64 encode for env var)
# macOS uses -b 0, Linux uses -w 0, so try both
if command -v base64 &> /dev/null; then
    # Try macOS style first
    PRIV_KEY=$(cat "$OUTPUT_DIR/$KEY_NAME" | base64 2>/dev/null)
    # If that didn't work (Linux), try with -w 0
    if [ -z "$PRIV_KEY" ]; then
        PRIV_KEY=$(cat "$OUTPUT_DIR/$KEY_NAME" | base64 -w 0 2>/dev/null)
    fi
fi

echo ""
echo "✅ SSH key pair generated successfully!"
echo ""
echo "Add these to your environment variables:"
echo ""
echo "export RECONCILER_SSH_PUBLIC_KEY=\"$PUB_KEY\""
echo ""
echo "export RECONCILER_SSH_PRIVATE_KEY=\"$PRIV_KEY\""
echo ""
echo "⚠️  IMPORTANT: Store the private key securely!"
echo "Raw key files saved to:"
echo "  - $OUTPUT_DIR/$KEY_NAME (private)"
echo "  - $OUTPUT_DIR/$KEY_NAME.pub (public)"
echo ""
echo "To use with Hetzner, the key will be automatically uploaded to Hetzner Cloud on first run."

