#!/bin/bash

# URL Shortener API Demo Script
# Demonstrates REST API endpoints with Swagger documentation

echo "🎬 URL Shortener REST API Demo"
echo "================================"
echo ""

# Check if services are running
echo "🔍 Checking if REST API service is running..."
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ REST API service is running"
else
    echo "❌ REST API service is not running"
    echo "💡 Start with: make run-rest"
    exit 1
fi

echo ""
echo "📖 API Documentation Available:"
echo "  📋 Interactive Swagger UI: http://localhost:8080/docs/index.html"
echo "  🏠 Documentation Home:     http://localhost:8080/"
echo "  📄 OpenAPI Specification:  http://localhost:8080/swagger/doc.json"
echo ""

echo "🧪 Testing REST API Endpoints:"
echo "==============================="

echo ""
echo "1️⃣ Health Check"
echo "----------------"
echo "GET /health"
curl -s http://localhost:8080/health | jq .
echo ""

echo "2️⃣ Create Short URL"
echo "--------------------"
echo "POST /api/v1/shorten"
SHORTEN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/shorten \
    -H "Content-Type: application/json" \
    -d '{
        "long_url": "https://github.com/golang/go",
        "user_id": "demo_user",
        "custom_alias": "golang",
        "metadata": {"campaign": "demo", "source": "api_test"}
    }')

echo "$SHORTEN_RESPONSE" | jq .

# Extract short_code for subsequent tests
SHORT_CODE=$(echo "$SHORTEN_RESPONSE" | jq -r '.short_code // "golang"')
echo ""

echo "3️⃣ Get URL Information"
echo "-----------------------"
echo "GET /api/v1/urls/$SHORT_CODE?user_id=demo_user"
curl -s "http://localhost:8080/api/v1/urls/$SHORT_CODE?user_id=demo_user" | jq .
echo ""

echo "4️⃣ List User URLs"
echo "------------------"
echo "GET /api/v1/users/demo_user/urls"
curl -s "http://localhost:8080/api/v1/users/demo_user/urls?page=1&page_size=10" | jq .
echo ""

echo "5️⃣ Update URL (if endpoint exists)"
echo "-----------------------------------"
echo "Note: Update functionality shown in Swagger UI"
echo ""

echo "6️⃣ Delete URL"
echo "--------------"
echo "DELETE /api/v1/urls/$SHORT_CODE?user_id=demo_user"
DELETE_RESPONSE=$(curl -s -X DELETE "http://localhost:8080/api/v1/urls/$SHORT_CODE?user_id=demo_user")
echo "$DELETE_RESPONSE" | jq .
echo ""

echo "✅ API Demo Complete!"
echo ""
echo "🔗 Next Steps:"
echo "  • Visit Swagger UI for interactive testing: http://localhost:8080/docs/index.html"
echo "  • Try different parameters and see real-time responses"
echo "  • Use 'Try it out' buttons in Swagger UI for easy testing"
echo "  • Export OpenAPI spec for API client generation"
echo ""

echo "📊 API Features Demonstrated:"
echo "  ✅ RESTful endpoint design"
echo "  ✅ JSON request/response format"
echo "  ✅ Error handling with proper HTTP status codes"
echo "  ✅ Query parameters and path variables"
echo "  ✅ Pagination support"
echo "  ✅ Metadata and custom alias support"
echo "  ✅ Interactive Swagger documentation"
echo ""

echo "🎯 For End Users:"
echo "  • This REST API (port 8080) is the public interface"
echo "  • Internal RPC service (port 50051) is not exposed"
echo "  • Complete API documentation available via Swagger UI" 