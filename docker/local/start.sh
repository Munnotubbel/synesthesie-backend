#!/bin/bash

echo "🚀 Starting Synesthesie Backend Services..."
echo ""
echo "📍 Services will be available at:"
echo "   - Backend API: http://localhost:8080"
echo "   - PostgreSQL: localhost:5433"
echo "   - Redis: localhost:6379"
echo ""
echo "🌐 Frontend is expected at: http://localhost:8081"
echo ""

# Start docker-compose
docker-compose up -d

# Check if services are running
echo ""
echo "⏳ Waiting for services to be healthy..."
sleep 5

echo ""
echo "📊 Service Status:"
docker-compose ps

echo ""
echo "📝 Logs: docker-compose logs -f"
echo "🛑 Stop: docker-compose down"
echo ""

echo "========================================="
echo "Development Environment is running:"
echo "   - API:          http://localhost:8080"
echo "   - PostgreSQL:   localhost:5433"
echo "   - Redis:        localhost:6379"
echo "========================================="
echo "Tailing logs... (Press Ctrl+C to stop)"
echo ""