# Technology Stack

**Analysis Date:** 2026-02-18

## Languages

**Primary:**
- Go 1.23 - Backend API, main application logic
- JavaScript - Package.json contains frontend dependencies (@use-gesture/react)

## Runtime

**Environment:**
- Go 1.24.4 (toolchain)
- Alpine Linux (Docker container)

**Package Manager:**
- Go Modules
- npm (for frontend components)
- Lockfile: go.sum present

## Frameworks

**Core:**
- Gin v1.9.1 - HTTP web framework for REST API
- GORM v1.25.5 - ORM for database operations
- GORM PostgreSQL Driver v1.5.4 - PostgreSQL integration

**Testing:**
- Go built-in testing framework

**Build/Dev:**
- Docker for containerization
- Make for build automation
- Alpine Linux for production containers

## Key Dependencies

**Critical:**
- github.com/golang-jwt/jwt/v5 v5.2.0 - JWT authentication
- github.com/redis/go-redis/v9 v9.3.1 - Redis client for caching
- github.com/stripe/stripe-go/v82 v82.3.0 - Stripe payment integration
- github.com/logpacker/PayPal-Go-SDK - PayPal payment integration
- github.com/aws/aws-sdk-go-v2 - AWS S3 for file storage
- golang.org/x/crypto v0.21.0 - Security utilities

**Infrastructure:**
- github.com/jung-kurt/gofpdf v1.16.2 - PDF generation
- github.com/skip2/go-qrcode v0.0.0 - QR code generation

## Configuration

**Environment:**
- .env file for configuration
- Environment variables for all services
- Multiple S3 configurations for different storage purposes

**Build:**
- Dockerfile for multi-stage builds
- docker-compose.yml for development environment
- Makefile for development workflow

## Platform Requirements

**Development:**
- Docker Compose for services
- PostgreSQL 16-alpine
- Redis 7-alpine
- Go 1.23+ compiler

**Production:**
- Linux container (Alpine)
- PostgreSQL database
- Redis cache
- Multiple S3-compatible storage services
- Port 8080 exposed

---

*Stack analysis: 2026-02-18*