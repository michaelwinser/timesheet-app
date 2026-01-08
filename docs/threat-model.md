# Threat Model: Timesheet Application

**Version:** 1.1
**Date:** 2026-01-07
**Classification:** Internal

---

## Executive Summary

This threat model analyzes the timesheet application, a self-hosted full-stack web application that automatically creates timesheets from Google Calendar events. The application handles sensitive data including:

- **Personal Identifiable Information (PII)**: Calendar events, attendee emails, work schedules
- **Financial data**: Billing rates, invoices, project accounting
- **Authentication credentials**: OAuth tokens, passwords, API keys

### Key Findings

| Risk Level | Count | Description |
|------------|-------|-------------|
| **High** | 4 | JWT secret defaults, CORS configuration, MCP redirect validation, default DB credentials |
| **Medium** | 8 | Token expiration, session management, input validation, dependency scanning, base images, build reproducibility |
| **Low** | 7 | Information disclosure, logging, rate limiting, container hardening, build pipeline |

**Categories:**
- Application Security: 12 findings
- Supply Chain: 7 findings

### Threat Actor Profile

Given the deployment context (personal self-hosted, behind Nginx Proxy Manager with HTTPS + basic auth), the primary threat actors are:

1. **Opportunistic attackers**: Scanning for exposed services, default credentials
2. **MCP clients (AI agents)**: User-trusted but application-untrusted per requirements
3. **Misconfiguration**: Accidental exposure of secrets or services

---

## System Overview

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Nginx Proxy Manager                          │
│                    (HTTPS + Basic Auth)                         │
└─────────────────────────┬───────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────┐
│                   Docker Container                               │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  Go Backend (chi router)                                 │    │
│  │  - REST API (/api/*)                                     │    │
│  │  - MCP Protocol (/mcp)                                   │    │
│  │  - Static files (SPA)                                    │    │
│  └────────────────────────┬────────────────────────────────┘    │
│                           │                                      │
│  ┌────────────────────────▼────────────────────────────────┐    │
│  │  PostgreSQL 16                                           │    │
│  │  - User data isolation                                   │    │
│  │  - Encrypted OAuth tokens                                │    │
│  └──────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────┘
                          │
                          ▼
            ┌─────────────────────────┐
            │  External Services      │
            │  - Google Calendar API  │
            │  - Google Sheets API    │
            │  - (Optional) Anthropic │
            └─────────────────────────┘
```

### Trust Boundaries

| Boundary | From | To | Notes |
|----------|------|-----|-------|
| **TB1** | Internet | Nginx Proxy Manager | Basic auth provides first layer |
| **TB2** | Nginx | Go Backend | HTTP (within Docker network) |
| **TB3** | Go Backend | PostgreSQL | Database credentials |
| **TB4** | Go Backend | Google APIs | OAuth tokens |
| **TB5** | MCP Client | Go Backend | MCP OAuth tokens |

### Data Classification

| Data Type | Sensitivity | Storage | Encryption |
|-----------|-------------|---------|------------|
| User passwords | High | PostgreSQL | bcrypt hashed |
| Google OAuth tokens | High | PostgreSQL | AES-256-GCM encrypted |
| API keys | High | PostgreSQL | SHA-256 hashed |
| MCP access tokens | High | PostgreSQL | SHA-256 hashed |
| JWT secret | Critical | Environment variable | N/A (must be kept secret) |
| Calendar events | Medium | PostgreSQL | Plaintext |
| Attendee emails | Medium-High (PII) | PostgreSQL | Plaintext |
| Billing rates | Medium | PostgreSQL | Plaintext |
| Invoices | Medium | PostgreSQL | Plaintext |

---

## Threat Analysis (STRIDE)

### 1. Spoofing

#### S-1: Default JWT Secret (HIGH)

**Location:** `service/cmd/server/main.go:28`

```go
jwtSecret := getEnv("JWT_SECRET", "development-secret-change-in-production")
```

**Threat:** If deployed with default JWT secret, attackers can forge valid JWTs for any user.

**Impact:** Complete authentication bypass, full account takeover.

**Likelihood:** Medium (requires knowing default secret, but it's in source code)

**Mitigation Status:**
- [ ] No runtime check that secret was changed
- [ ] No minimum length validation

**Recommendations:**
1. Fail startup if JWT_SECRET equals default value
2. Require minimum 32-character secret
3. Log warning if secret appears weak

---

#### S-2: MCP Client Impersonation (MEDIUM)

**Location:** `service/internal/handler/mcp_oauth.go:82-84`

```go
// Generate a simple client ID (for public clients using PKCE, no secret needed)
clientID := fmt.Sprintf("mcp_%d", time.Now().UnixNano())
```

**Threat:** Dynamic client registration accepts any client without validation. While PKCE protects the token exchange, the client registration is open.

**Impact:** Any MCP client can register. This is by design for MCP but noted for awareness.

**Likelihood:** Low (PKCE still required for tokens)

**Current Controls:**
- [x] PKCE required (S256)
- [x] User must authenticate via login form
- [x] Tokens scoped to authenticated user

**Recommendations:** This is acceptable given PKCE requirements. Document that all MCP clients have equivalent access to API.

---

#### S-3: Session Fixation via OAuth State (LOW)

**Location:** `service/internal/store/mcp_oauth.go:122-149`

**Threat:** OAuth state is generated server-side and stored in DB. No fixation risk identified.

**Current Controls:**
- [x] State generated with crypto/rand (32 bytes)
- [x] State stored server-side, not client-controlled
- [x] Sessions expire in 10 minutes

---

### 2. Tampering

#### T-1: Classification Rule Injection (MEDIUM)

**Location:** `service/internal/classification/parser.go`

**Threat:** User-defined classification rules use a Gmail-style query language. Maliciously crafted rules could potentially cause issues.

**Impact:** Unlikely to cause data tampering beyond user's own data (user isolation enforced). Possible regex DoS with complex patterns.

**Current Controls:**
- [x] Rules scoped to user_id
- [x] Database constraints enforce ownership

**Recommendations:**
1. Validate regex patterns for complexity (avoid catastrophic backtracking)
2. Set timeout on rule evaluation

---

#### T-2: Time Entry Manipulation via MCP (MEDIUM)

**Location:** MCP tools expose time entry creation/modification

**Threat:** MCP clients can create, modify, or delete time entries. Per stated requirements, MCP should have same access as API.

**Current Controls:**
- [x] MCP tokens scoped to authenticated user
- [x] Same authorization as API endpoints
- [x] User must explicitly authorize MCP client

**Status:** Working as designed. MCP clients are user-trusted.

---

### 3. Repudiation

#### R-1: Insufficient Audit Logging (LOW)

**Location:** Application-wide

**Threat:** Limited audit trail for security-relevant actions. Only `classification_history` table tracks classification decisions.

**Missing Audit Events:**
- User login/logout
- API key creation/deletion
- MCP token issuance
- OAuth connection changes
- Invoice creation/modification

**Impact:** Difficulty investigating security incidents.

**Recommendations:**
1. Add structured security event logging
2. Consider audit table for sensitive operations

---

### 4. Information Disclosure

#### I-1: OpenAPI Spec Exposure (LOW)

**Location:** `service/cmd/server/main.go:166-169`

```go
r.Get("/api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, apiSpecPath)
})
```

**Threat:** Full API specification publicly accessible, revealing all endpoints and data structures.

**Impact:** Reconnaissance aid for attackers.

**Mitigation:** Already behind basic auth at Nginx level.

**Recommendations:** Consider if spec needs to be served in production.

---

#### I-2: Detailed Error Messages (LOW)

**Location:** Various handlers

**Threat:** Some error responses may leak implementation details.

**Example:** `service/internal/handler/auth.go:104-109` returns generic "Email or password is incorrect" (good practice).

**Current Controls:**
- [x] Auth errors are generic
- [x] Internal errors logged, not returned to client

---

#### I-3: Calendar Data Contains PII (MEDIUM)

**Location:** `events` table stores attendee emails

**Threat:** Calendar events include attendee email addresses (PII). This data is:
- Stored in plaintext
- Accessible via API
- Exposed to MCP clients

**Impact:** Privacy concern if database compromised.

**Current Controls:**
- [x] User isolation enforced
- [x] Database on local Docker network

**Recommendations:**
1. Document that attendee emails are stored
2. Consider data retention policy for old events

---

### 5. Denial of Service

#### D-1: No Rate Limiting (MEDIUM)

**Location:** Application-wide

**Threat:** No rate limiting on API endpoints. An attacker (or misconfigured MCP client) could exhaust resources.

**Impact:** Service unavailability, database overload.

**Current Controls:**
- [x] Basic auth at Nginx level limits unauthenticated access
- [ ] No application-level rate limiting

**Recommendations:**
1. Add rate limiting middleware (chi has plugins)
2. Consider per-user rate limits for API key/MCP access

---

#### D-2: Unbounded Calendar Sync (LOW)

**Location:** `service/internal/google/calendar.go`

**Threat:** Full calendar sync could pull large amounts of data.

**Current Controls:**
- [x] Incremental sync preferred
- [x] User-initiated only

---

#### D-3: Regex Complexity in Rules (MEDIUM)

**Location:** `service/internal/classification/evaluator.go`

**Threat:** User-defined regex patterns could cause ReDoS (Regular Expression Denial of Service).

**Recommendations:**
1. Use regex timeout or complexity limits
2. Consider using RE2 (linear time guarantees)

---

### 6. Elevation of Privilege

#### E-1: CORS Allows Any Origin (HIGH)

**Location:** `service/cmd/server/main.go:126-137`

```go
w.Header().Set("Access-Control-Allow-Origin", "*")
```

**Threat:** Wildcard CORS allows any website to make authenticated requests if user has valid credentials stored.

**Impact:** Cross-site request forgery from any origin.

**Current Controls:**
- [x] Basic auth at Nginx level (cookies not passed through basic auth)
- [ ] No CSRF protection at application level

**Recommendations:**
1. Restrict CORS to specific origins in production
2. At minimum, use the actual frontend origin

---

#### E-2: MCP Redirect URI Not Validated (HIGH)

**Location:** `service/internal/handler/mcp_oauth.go:124-128`

```go
if redirectURI == "" {
    http.Error(w, "redirect_uri is required", http.StatusBadRequest)
    return
}
// No validation of redirect_uri domain/scheme
```

**Threat:** Authorization code could be redirected to attacker-controlled URL.

**Impact:** Token theft if user is tricked into authorizing malicious client.

**Attack Scenario:**
1. Attacker registers client with `redirect_uri=https://evil.com/callback`
2. User is tricked into authorizing
3. Auth code sent to attacker
4. Attacker exchanges code for token (if they have PKCE verifier)

**Mitigating Factor:** PKCE means attacker needs the code_verifier to exchange the code. However, best practice is to still validate redirect URIs.

**Recommendations:**
1. Maintain allowlist of permitted redirect URI patterns
2. At minimum, warn user of redirect destination in consent screen
3. Consider requiring localhost or specific domains for MCP clients

---

#### E-3: User ID from Context Not Validated (LOW)

**Location:** `service/internal/handler/middleware.go:23-27`

```go
if authHeader == "" {
    next.ServeHTTP(w, r)
    return
}
```

**Threat:** Missing auth header passes through (relies on handlers to check).

**Current Controls:**
- [x] Handlers check `UserIDFromContext()` and return 401
- [x] Database queries include user_id in WHERE clauses

**Status:** Defense in depth working correctly.

---

## Authentication & Session Security

### Password Security

**Location:** `service/internal/store/users.go:42-46`

```go
hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
```

**Assessment:**
- [x] bcrypt with default cost (10)
- [x] Minimum 8 character password required
- [x] Generic error for invalid credentials (no user enumeration)

**Recommendations:**
1. Consider increasing bcrypt cost to 12
2. Add password complexity requirements (optional for personal use)

---

### JWT Security

**Location:** `service/internal/handler/jwt.go`

**Assessment:**
- [x] HS256 signing (acceptable for single-service)
- [x] 24-hour expiration
- [x] Validates algorithm in ParseWithClaims
- [ ] No token blacklist for logout (comment acknowledges this)

**Recommendations:**
1. For logout to be effective, implement token blacklist or reduce expiration
2. Consider adding `jti` claim for future revocation support

---

### API Key Security

**Location:** `service/internal/store/api_keys.go`

**Assessment:**
- [x] SHA-256 hashed (not reversible)
- [x] 256-bit random key
- [x] Prefix stored for identification (`ts_`)
- [x] User-scoped

**Status:** Good implementation.

---

### MCP OAuth Security

**Location:** `service/internal/store/mcp_oauth.go`

**Assessment:**
- [x] PKCE required (S256 only)
- [x] Auth codes single-use (deleted after exchange)
- [x] Auth codes expire in 5 minutes
- [x] Sessions expire in 10 minutes
- [x] Tokens expire in 24 hours
- [x] Tokens hashed in database

**Status:** Solid OAuth 2.1 implementation with PKCE.

---

### Google OAuth Token Security

**Location:** `service/internal/crypto/encryption.go`

**Assessment:**
- [x] AES-256-GCM encryption
- [x] Random nonce per encryption
- [x] 32-byte key required

**Status:** Good encryption implementation.

---

## Infrastructure Security

### Docker Configuration

**Assessed from exploration, not direct file review:**
- Non-root user in container (good)
- Multi-stage build (good, smaller attack surface)
- Default database credentials in compose (must change)

---

### Environment Variables

| Variable | Risk if Exposed | Current Protection |
|----------|-----------------|-------------------|
| `JWT_SECRET` | Critical - auth bypass | Environment only |
| `ENCRYPTION_KEY` | High - decrypt OAuth tokens | Environment only |
| `DATABASE_URL` | High - data access | Environment only |
| `GOOGLE_CLIENT_SECRET` | Medium - OAuth abuse | Environment only |

**Recommendations:**
1. Use secrets management in production
2. Document required secret rotation procedures

---

### Database Security

**Assessment:**
- [x] User isolation via user_id in all tables
- [x] Foreign keys with ON DELETE CASCADE
- [x] Indexes for performance
- [ ] No row-level security (RLS) - acceptable for single-user

---

## MCP-Specific Threats

### Assumption Documentation

Per requirements, MCP clients are:
- **User-trusted**: The user explicitly authorizes the MCP client
- **Application-untrusted**: MCP should not have elevated privileges over API

### MCP Threat Analysis

| Threat | Status | Notes |
|--------|--------|-------|
| MCP client accesses other users' data | Mitigated | Token scoped to user_id |
| MCP client performs destructive actions | Accepted | Same as API access |
| MCP client leaks data to third parties | User responsibility | User trusts their AI agent |
| MCP client makes excessive requests | Unmitigated | No rate limiting |
| MCP redirect to malicious URI | Partially mitigated | PKCE helps, but URI not validated |

### MCP-Specific Recommendations

1. **Rate limiting**: Implement per-token rate limits
2. **Scope limitations**: Consider read-only vs read-write MCP scopes
3. **Token revocation UI**: Allow users to revoke MCP tokens
4. **Activity logging**: Log MCP tool invocations

---

## Supply Chain & Dependency Threats

### Overview

The application relies on external dependencies across multiple ecosystems:

| Ecosystem | Direct Dependencies | Transitive | Package Manager |
|-----------|---------------------|------------|-----------------|
| Go | 9 | 37 | go mod |
| Node.js | 12 (dev only) | ~200+ | npm |
| Docker | 4 base images | N/A | Docker Hub |
| Database | 1 | N/A | Docker Hub |

### SC-1: Go Dependencies (MEDIUM)

**Direct dependencies (go.mod):**

| Package | Purpose | Risk Notes |
|---------|---------|------------|
| `github.com/go-chi/chi/v5` | HTTP router | Well-maintained, popular |
| `github.com/golang-jwt/jwt/v5` | JWT handling | Security-critical, actively maintained |
| `github.com/jackc/pgx/v5` | PostgreSQL driver | Well-maintained, popular |
| `github.com/google/uuid` | UUID generation | Google-maintained |
| `golang.org/x/crypto` | bcrypt, crypto | Go team maintained |
| `golang.org/x/oauth2` | OAuth client | Go team maintained |
| `google.golang.org/api` | Google APIs | Google-maintained |
| `github.com/getkin/kin-openapi` | OpenAPI parsing | Code generation tool |
| `github.com/oapi-codegen/runtime` | OpenAPI runtime | Code generation tool |

**Assessment:**
- [x] All direct deps are well-known, maintained packages
- [x] Security-critical packages (jwt, crypto) from reputable sources
- [x] Go's checksum database provides integrity verification
- [ ] No automated vulnerability scanning configured

**Transitive dependency concerns:**
- `google.golang.org/grpc` - Large dependency tree from Google APIs
- OpenTelemetry packages pulled in (unused but present)

**Recommendations:**
1. Run `govulncheck` periodically to scan for known vulnerabilities
2. Consider `go mod tidy` to remove unused dependencies
3. Pin to specific versions in production builds

---

### SC-2: Node.js Dependencies (LOW)

**Direct dependencies (package.json - all devDependencies):**

| Package | Purpose | Risk Notes |
|---------|---------|------------|
| `@sveltejs/kit` | Framework | Well-maintained |
| `svelte` | UI framework | Well-maintained |
| `vite` | Build tool | Well-maintained |
| `tailwindcss` | CSS framework | Well-maintained |
| `typescript` | Type checking | Microsoft-maintained |
| `@sveltejs/adapter-static` | Static build | Part of SvelteKit |

**Assessment:**
- [x] All devDependencies (not shipped to production)
- [x] Build output is static HTML/JS/CSS
- [x] Well-known, popular packages
- [ ] Large transitive dependency tree (~200+ packages typical for Node.js)
- [ ] No `package-lock.json` committed (seen `package-lock.json*` glob in Dockerfile)

**Risk Mitigation:**
- Node.js dependencies only affect build, not runtime
- Static output means no server-side Node.js code in production
- Supply chain attack would need to compromise build output

**Recommendations:**
1. Commit `package-lock.json` for reproducible builds
2. Run `npm audit` periodically
3. Consider using `npm ci` instead of `npm install` in Dockerfile

---

### SC-3: Docker Base Images (MEDIUM)

**Images used:**

| Image | Stage | Purpose | Risk |
|-------|-------|---------|------|
| `node:20-alpine` | Build | Frontend build | Medium - large image |
| `golang:1.24-alpine` | Build | Backend build | Medium - large image |
| `alpine:3.20` | Runtime | Production | Low - minimal image |
| `postgres:16-alpine` | Database | PostgreSQL | Medium - database |
| `python:3.11-slim` | Legacy | Old Dockerfile | Not used in current stack |

**Assessment:**
- [x] Alpine-based images (smaller attack surface)
- [x] Multi-stage builds (build tools not in runtime)
- [x] Specific version tags (not `latest`)
- [ ] No image signature verification
- [ ] No automated base image updates

**Security considerations:**
- `alpine:3.20` is minimal but still needs periodic updates
- Build images have more attack surface but aren't deployed
- PostgreSQL image should be kept updated for security patches

**Recommendations:**
1. Enable Docker Content Trust for image verification
2. Set up automated base image vulnerability scanning (e.g., Trivy, Snyk)
3. Create process for periodic base image updates
4. Consider distroless images for runtime (more minimal than Alpine)

---

### SC-4: Runtime Missing from Container (LOW)

**Location:** `service/Dockerfile:37`

```dockerfile
FROM alpine:3.20
```

**Assessment:**
The runtime container includes only:
- Go binary (statically linked)
- CA certificates
- Static web assets

**Positive:**
- [x] No shell beyond Alpine's default
- [x] CGO disabled (pure Go binary)
- [x] Minimal packages installed

**Recommendations:**
1. Consider adding non-root user (currently missing in service/Dockerfile)
2. Root Dockerfile has non-root user, service/Dockerfile does not - inconsistency

---

### SC-5: Database Default Credentials (HIGH)

**Location:** `docker-compose.yaml:17-18`

```yaml
POSTGRES_USER: timesheet
POSTGRES_PASSWORD: changeMe123!
```

**Threat:** Default credentials in version control. Even for development, these could be accidentally used in production.

**Impact:** Database compromise if exposed.

**Recommendations:**
1. Use environment variables for all credentials in docker-compose
2. Add `.env.example` with placeholder values
3. Document credential rotation requirements

---

### SC-6: No Dependency Pinning Strategy (MEDIUM)

**Go:**
- Uses `go.mod` with minimum versions
- `go.sum` provides checksums (good)
- No maximum version constraints

**Node.js:**
- Uses `^` version ranges (e.g., `^5.0.0`)
- `package-lock.json` may not be committed
- `npm install` in Dockerfile could get different versions

**Recommendations:**
1. Consider using exact versions for security-critical Go deps
2. Commit and use `package-lock.json`
3. Use `npm ci` instead of `npm install` for reproducible builds

---

### SC-7: Build Pipeline Integrity (LOW)

**Current state:**
- Local builds via Makefile/docker-compose
- No CI/CD pipeline visible in repository
- No automated security scanning

**For future CI/CD:**

| Control | Status | Notes |
|---------|--------|-------|
| Signed commits | Unknown | Recommended for supply chain |
| Dependency scanning | Missing | Add govulncheck, npm audit |
| Container scanning | Missing | Add Trivy or similar |
| SBOM generation | Missing | Recommended for tracking |
| Build reproducibility | Partial | Lock files needed |

---

### Supply Chain Threat Summary

| ID | Threat | Severity | Likelihood | Risk | Status |
|----|--------|----------|------------|------|--------|
| SC-5 | Default DB credentials | High | Medium | **HIGH** | Unmitigated |
| SC-1 | Go dependency vulnerabilities | Medium | Low | **MEDIUM** | No scanning |
| SC-3 | Outdated base images | Medium | Medium | **MEDIUM** | No automation |
| SC-6 | Non-reproducible builds | Medium | Low | **MEDIUM** | Partial |
| SC-2 | Node.js build dependencies | Low | Low | **LOW** | Build-only |
| SC-4 | Container hardening | Low | Low | **LOW** | Mostly good |
| SC-7 | Build pipeline integrity | Low | Low | **LOW** | Local builds |

---

### Supply Chain Recommendations

**Immediate:**
1. Remove default credentials from docker-compose.yaml
2. Commit `package-lock.json`
3. Add non-root user to `service/Dockerfile`

**Short-term:**
4. Set up `govulncheck` for Go dependency scanning
5. Set up `npm audit` for Node.js scanning
6. Add container image scanning (Trivy)

**Medium-term:**
7. Generate SBOM (Software Bill of Materials) for deployments
8. Consider signing container images
9. Set up Dependabot or Renovate for automated dependency updates

---

## Risk Summary Matrix

### Application Security Threats

| ID | Threat | Severity | Likelihood | Risk | Status |
|----|--------|----------|------------|------|--------|
| S-1 | Default JWT secret | Critical | Medium | **HIGH** | Unmitigated |
| E-1 | CORS wildcard | High | Medium | **HIGH** | Unmitigated |
| E-2 | MCP redirect not validated | High | Low | **HIGH** | Partially mitigated (PKCE) |
| T-1 | Rule injection/ReDoS | Medium | Medium | **MEDIUM** | Unmitigated |
| D-1 | No rate limiting | Medium | Medium | **MEDIUM** | Unmitigated |
| D-3 | Regex complexity | Medium | Medium | **MEDIUM** | Unmitigated |
| I-3 | PII in calendar data | Medium | Low | **MEDIUM** | Accepted |
| R-1 | Insufficient audit logging | Low | High | **MEDIUM** | Unmitigated |
| S-2 | MCP client registration open | Low | Low | **LOW** | Accepted |
| I-1 | OpenAPI exposure | Low | Medium | **LOW** | Mitigated (basic auth) |
| I-2 | Error message details | Low | Low | **LOW** | Mostly mitigated |
| D-2 | Unbounded calendar sync | Low | Low | **LOW** | Acceptable |

### Supply Chain Threats

| ID | Threat | Severity | Likelihood | Risk | Status |
|----|--------|----------|------------|------|--------|
| SC-5 | Default DB credentials in VCS | High | Medium | **HIGH** | Unmitigated |
| SC-1 | Go dependency vulnerabilities | Medium | Low | **MEDIUM** | No scanning |
| SC-3 | Outdated Docker base images | Medium | Medium | **MEDIUM** | No automation |
| SC-6 | Non-reproducible builds | Medium | Low | **MEDIUM** | Partial |
| SC-2 | Node.js build dependencies | Low | Low | **LOW** | Build-only risk |
| SC-4 | Container runs as root | Low | Low | **LOW** | Missing non-root user |
| SC-7 | Build pipeline integrity | Low | Low | **LOW** | Local builds only |

---

## Recommended Remediation Priority

### Immediate (Before Production)

1. **S-1**: Add startup validation for JWT_SECRET
   - Fail if equals default value
   - Warn if < 32 characters

2. **E-1**: Configure CORS properly
   - Set specific origin instead of `*`
   - Consider environment variable for allowed origins

3. **SC-5**: Remove default credentials from docker-compose.yaml
   - Use environment variables with `.env` file
   - Add `.env.example` with placeholders

### Short-term

4. **E-2**: Add redirect URI validation
   - Allowlist of permitted schemes (http://localhost:*, https://*)
   - Display redirect destination to user before authorization

5. **D-1/D-3**: Add rate limiting
   - Global rate limit
   - Per-user/per-token limits
   - Regex evaluation timeout

6. **SC-6**: Improve build reproducibility
   - Commit `package-lock.json`
   - Use `npm ci` instead of `npm install` in Dockerfile

7. **SC-4**: Add non-root user to service/Dockerfile
   - Align with root Dockerfile pattern

### Medium-term

8. **R-1**: Implement security audit logging
   - Authentication events
   - Token lifecycle events
   - Administrative actions

9. **T-1**: Validate classification rule patterns
   - Regex complexity analysis
   - Consider using RE2

10. **SC-1/SC-3**: Set up dependency and image scanning
    - `govulncheck` for Go dependencies
    - `npm audit` for Node.js
    - Trivy or similar for container images
    - Consider Dependabot/Renovate for automated updates

---

## Appendix A: Security Configuration Checklist

### Before Deploying to Production

**Secrets & Credentials:**
- [ ] Changed `JWT_SECRET` from default (minimum 32 chars, random)
- [ ] Set `ENCRYPTION_KEY` (64 hex chars = 32 bytes)
- [ ] Changed database password from default
- [ ] Created `.env` file (not committed) with all secrets
- [ ] Google OAuth credentials configured

**Network & Access:**
- [ ] HTTPS enabled (via Nginx Proxy Manager)
- [ ] CORS configured with specific origin (not `*`)
- [ ] Basic auth configured (optional, may conflict with MCP)
- [ ] Reviewed MCP access grants

**Supply Chain:**
- [ ] `package-lock.json` committed
- [ ] Dockerfile uses non-root user
- [ ] Base images are recent versions
- [ ] Ran `govulncheck` on Go dependencies
- [ ] Ran `npm audit` on Node.js dependencies

### Periodic Maintenance

- [ ] Update Docker base images monthly
- [ ] Review and update dependencies quarterly
- [ ] Rotate secrets annually (or after any suspected compromise)
- [ ] Review MCP token grants

---

## Appendix B: Threat Model Assumptions

1. **Deployment context**: Self-hosted on TrueNAS, behind Nginx Proxy Manager
2. **Network exposure**: Internet-accessible via reverse proxy
3. **User base**: Primarily single user, personal use
4. **MCP trust model**: Users trust their AI agents but application enforces same limits as API
5. **Basic auth**: May need to be disabled for MCP clients to connect directly

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-07 | Claude (threat model) | Initial threat model |
| 1.1 | 2026-01-07 | Claude (threat model) | Added supply chain & dependency analysis |
