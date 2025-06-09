#!/bin/bash

# URL Shortener Testing Guide
# This script provides clear guidance on testing the URL Shortener microservices

echo "🎯 URL Shortener Testing Guide"
echo "================================"
echo ""

echo "🏗️ Architecture Overview:"
echo "┌─────────────────┐    Go Micro RPC    ┌─────────────────┐"
echo "│   REST API      │ ←─── over NATS ──→ │   RPC Service   │"
echo "│   Service       │                    │   (Internal)    │"
echo "│   Port: 8080    │                    │   Port: 50051   │"
echo "│ (PUBLIC FACING) │                    │   (PRIVATE)     │"
echo "└─────────────────┘                    └─────────────────┘"
echo ""

echo "🌐 What End Users Access:"
echo "  ✅ REST API Service (Port 8080) - HTTP endpoints"
echo "  ❌ RPC Service (Port 50051) - Internal Go Micro RPC (NOT gRPC!)"
echo ""

echo "📋 Testing Commands:"
echo ""

echo "1️⃣ For END USERS (Testing REST API):"
echo "   make demo-api                    # Show available endpoints"
echo "   make test-api                    # Basic API test"
echo "   make test-api-comprehensive      # Full API test suite"
echo ""

echo "2️⃣ For DEVELOPERS (Internal testing):"
echo "   make test                        # Unit tests (26/26 tests)"
echo "   make test-rpc-internal          # Info about internal RPC"
echo "   make health                     # Check service health"
echo ""

echo "3️⃣ Manual Testing Examples:"
echo ""

echo "🔍 Health Check:"
echo "curl http://localhost:8080/health"
echo ""

echo "📝 Create Short URL:"
echo "curl -X POST http://localhost:8080/api/v1/shorten \\"
echo "  -H 'Content-Type: application/json' \\"
echo "  -d '{\"long_url\":\"https://github.com\",\"user_id\":\"user123\",\"custom_alias\":\"github\"}'"
echo ""

echo "🔍 Get URL Info:"
echo "curl 'http://localhost:8080/api/v1/urls/github?user_id=user123'"
echo ""

echo "📋 List User URLs:"
echo "curl 'http://localhost:8080/api/v1/users/user123/urls'"
echo ""

echo "🗑️ Delete URL:"
echo "curl -X DELETE 'http://localhost:8080/api/v1/urls/github?user_id=user123'"
echo ""

echo "⚠️  Important Notes:"
echo "• Only REST API (port 8080) is exposed to end users"
echo "• RPC Service (port 50051) is for internal microservice communication"
echo "• RPC uses Go Micro over NATS (NOT gRPC)"
echo "• Both services must be running for the system to work"
echo ""

echo "🚀 Start Services:"
echo "  Terminal 1: make run-rpc    # Start internal RPC service"
echo "  Terminal 2: make run-rest   # Start public REST API"
echo "  Terminal 3: make test-api   # Test the public API" 