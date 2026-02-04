#!/bin/bash

# TrustLink Backend - Digital Ocean Deployment Script
# This script deploys the TrustLink backend to a Digital Ocean droplet

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================="
echo "  TrustLink Backend Deployment Script"
echo "========================================="
echo ""

# Configuration
DROPLET_IP="${DROPLET_IP:-}"
DROPLET_USER="${DROPLET_USER:-root}"
DEPLOY_PATH="${DEPLOY_PATH:-/opt/trustlink}"
REPO_URL="${REPO_URL:-}"

# Functions
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

check_prerequisites() {
    print_info "Checking prerequisites..."
    
    # Check if ssh is available
    if ! command -v ssh &> /dev/null; then
        print_error "SSH is not installed"
        exit 1
    fi
    
    # Check if scp is available
    if ! command -v scp &> /dev/null; then
        print_error "SCP is not installed"
        exit 1
    fi
    
    # Check if firebase credentials exist
    if [ ! -f "./credentials/firebase-key.json" ]; then
        print_error "Firebase credentials not found at ./credentials/firebase-key.json"
        exit 1
    fi
    
    print_success "Prerequisites check passed"
}

get_droplet_info() {
    if [ -z "$DROPLET_IP" ]; then
        read -p "Enter your Digital Ocean Droplet IP: " DROPLET_IP
    fi
    
    if [ -z "$REPO_URL" ]; then
        read -p "Enter your Git repository URL (or press Enter to skip): " REPO_URL
    fi
    
    echo ""
    print_info "Deployment Configuration:"
    echo "  Droplet IP: $DROPLET_IP"
    echo "  User: $DROPLET_USER"
    echo "  Deploy Path: $DEPLOY_PATH"
    echo "  Repo URL: ${REPO_URL:-Local files}"
    echo ""
    
    read -p "Continue with deployment? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_error "Deployment cancelled"
        exit 1
    fi
}

install_docker() {
    print_info "Installing Docker on droplet..."
    
    ssh $DROPLET_USER@$DROPLET_IP <<'ENDSSH'
        # Update package lists
        sudo apt-get update
        
        # Install Docker if not already installed
        if ! command -v docker &> /dev/null; then
            curl -fsSL https://get.docker.com -o get-docker.sh
            sudo sh get-docker.sh
            sudo usermod -aG docker $USER
            rm get-docker.sh
        fi
        
        # Install Docker Compose plugin
        sudo apt-get install -y docker-compose-plugin
        
        # Verify installation
        docker --version
        docker compose version
ENDSSH
    
    print_success "Docker installed successfully"
}

setup_firewall() {
    print_info "Configuring firewall..."
    
    ssh $DROPLET_USER@$DROPLET_IP <<'ENDSSH'
        # Install UFW if not present
        sudo apt-get install -y ufw
        
        # Reset UFW to defaults
        sudo ufw --force reset
        
        # Allow SSH (important!)
        sudo ufw allow 22/tcp
        
        # Allow HTTP and HTTPS
        sudo ufw allow 80/tcp
        sudo ufw allow 443/tcp
        
        # Enable firewall
        sudo ufw --force enable
        
        # Show status
        sudo ufw status
ENDSSH
    
    print_success "Firewall configured"
}

deploy_code() {
    print_info "Deploying code to droplet..."
    
    # Create deployment directory
    ssh $DROPLET_USER@$DROPLET_IP "sudo mkdir -p $DEPLOY_PATH && sudo chown $DROPLET_USER:$DROPLET_USER $DEPLOY_PATH"
    
    if [ -n "$REPO_URL" ]; then
        # Clone from repository
        ssh $DROPLET_USER@$DROPLET_IP "cd $DEPLOY_PATH && git clone $REPO_URL backend || (cd backend && git pull)"
    else
        # Copy local files
        print_info "Copying local files to droplet..."
        rsync -avz --exclude 'credentials/*.json' --exclude '*.log' --exclude 'nohup.out' \
            ./ $DROPLET_USER@$DROPLET_IP:$DEPLOY_PATH/backend/
    fi
    
    print_success "Code deployed"
}

transfer_credentials() {
    print_info "Transferring Firebase credentials..."
    
    # Create credentials directory on droplet
    ssh $DROPLET_USER@$DROPLET_IP "mkdir -p $DEPLOY_PATH/backend/credentials"
    
    # Transfer credentials securely
    scp ./credentials/firebase-key.json $DROPLET_USER@$DROPLET_IP:$DEPLOY_PATH/backend/credentials/firebase-key.json
    
    # Set proper permissions
    ssh $DROPLET_USER@$DROPLET_IP "chmod 600 $DEPLOY_PATH/backend/credentials/firebase-key.json"
    
    print_success "Credentials transferred securely"
}

build_and_start_services() {
    print_info "Building and starting Docker containers..."
    
    ssh $DROPLET_USER@$DROPLET_IP <<ENDSSH
        cd $DEPLOY_PATH/backend
        
        # Build Docker images
        docker compose build --no-cache
        
        # Start services
        docker compose up -d
        
        # Wait for services to start
        sleep 10
        
        # Show status
        docker compose ps
ENDSSH
    
    print_success "Services started"
}

verify_deployment() {
    print_info "Verifying deployment..."
    
    sleep 5
    
    # Test health endpoint
    if curl -s -k "https://$DROPLET_IP/health" | grep -q "OK"; then
        print_success "Caddy health check passed"
    else
        print_error "Caddy health check failed"
    fi
    
    # Test gateway endpoint
    if curl -s -k "https://$DROPLET_IP/healthz" | grep -q "ok"; then
        print_success "Gateway health check passed"
    else
        print_error "Gateway health check failed"
    fi
    
    # Test profile endpoint (should return 401)
    HTTP_CODE=$(curl -s -k -o /dev/null -w "%{http_code}" "https://$DROPLET_IP/v1/profile/me")
    if [ "$HTTP_CODE" == "401" ]; then
        print_success "Profile service responding correctly (401 Unauthorized)"
    else
        print_error "Profile service check failed (expected 401, got $HTTP_CODE)"
    fi
}

show_completion_info() {
    echo ""
    echo "========================================="
    echo "  Deployment Complete!"
    echo "========================================="
    echo ""
    print_success "Backend is now running on your droplet!"
    echo ""
    echo "API Endpoints:"
    echo "  Health: https://$DROPLET_IP/health"
    echo "  Gateway: https://$DROPLET_IP/healthz"
    echo "  Profile: https://$DROPLET_IP/v1/profile/me"
    echo ""
    echo "Update your Flutter app:"
    echo "  ApiConstants.baseUrl = \"https://$DROPLET_IP\";"
    echo ""
    echo "View logs:"
    echo "  ssh $DROPLET_USER@$DROPLET_IP 'cd $DEPLOY_PATH/backend && docker compose logs -f'"
    echo ""
    echo "Manage services:"
    echo "  Stop: ssh $DROPLET_USER@$DROPLET_IP 'cd $DEPLOY_PATH/backend && docker compose down'"
    echo "  Start: ssh $DROPLET_USER@$DROPLET_IP 'cd $DEPLOY_PATH/backend && docker compose up -d'"
    echo "  Restart: ssh $DROPLET_USER@$DROPLET_IP 'cd $DEPLOY_PATH/backend && docker compose restart'"
    echo ""
}

# Main deployment flow
main() {
    check_prerequisites
    get_droplet_info
    install_docker
    setup_firewall
    deploy_code
    transfer_credentials
    build_and_start_services
    verify_deployment
    show_completion_info
}

# Run main function
main
