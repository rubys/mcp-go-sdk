# Test Coverage Enhancement Plan for Go MCP SDK

Based on comprehensive analysis of the TypeScript MCP SDK test suite, this document outlines gaps in our Go SDK test coverage and provides a plan to achieve feature parity.

## Executive Summary

The Go SDK has strong test coverage (24 test files, 2,600+ lines) but lacks several advanced test scenarios present in the TypeScript SDK. Key gaps include URI template support, advanced progress notification scenarios, protocol version negotiation, and transport resumability.

## Missing Test Coverage Areas

### 1. **URI Template Support** (Priority: HIGH)
The TypeScript SDK includes comprehensive URI template testing (`uriTemplate.test.ts`) that we completely lack.

**Required Tests:**
- [ ] Basic URI template expansion (`{var}` → value)
- [ ] Multiple variable expansion (`{x,y}` → `value1,value2`)
- [ ] Query parameter expansion (`{?q,limit}`)
- [ ] Path segment expansion (`{/path*}`)
- [ ] Fragment expansion (`{#section}`)
- [ ] Reserved character encoding
- [ ] Variable detection and extraction
- [ ] Edge cases (empty values, missing variables)

**Implementation Location:** `shared/uri_template_test.go`

### 2. **Advanced Progress Notification Scenarios** (Priority: HIGH)
While we support progress notifications, we lack the comprehensive timeout and edge case testing from TypeScript's `protocol.test.ts`.

**Required Tests:**
- [ ] Progress with timeout reset (`resetTimeoutOnProgress`)
- [ ] Maximum total timeout enforcement (`maxTotalTimeout`)
- [ ] Zero timeout handling
- [ ] Multiple rapid progress updates
- [ ] Progress without callback (no token generation)
- [ ] Near-timeout progress scenarios
- [ ] Progress token inclusion in `_meta` field
- [ ] Preserving existing `_meta` fields

**Implementation Location:** `internal/progress_test.go`

### 3. **Protocol Version Negotiation** (Priority: HIGH)
TypeScript SDK has extensive version negotiation tests that we lack.

**Required Tests:**
- [ ] Matching protocol versions
- [ ] Version mismatch handling
- [ ] Fallback to supported versions
- [ ] Multiple supported versions
- [ ] Version upgrade/downgrade scenarios
- [ ] Invalid version rejection

**Implementation Location:** `shared/protocol_version_test.go`

### 4. **WebSocket Transport** (Priority: MEDIUM)
TypeScript SDK includes WebSocket transport that we don't implement.

**Required Implementation:**
- [ ] WebSocket transport layer
- [ ] Connection management
- [ ] Reconnection logic
- [ ] Message framing
- [ ] Concurrent message handling

**Implementation Location:** `transport/websocket.go` and `transport/websocket_test.go`

### 5. **Transport Resumability** (Priority: MEDIUM)
TypeScript's `taskResumability.test.ts` tests advanced scenarios we lack.

**Required Tests:**
- [ ] Connection interruption and resume
- [ ] State persistence across reconnects
- [ ] In-flight request handling
- [ ] Event store integration
- [ ] Session ID tracking

**Implementation Location:** `transport/resumable_test.go`

### 6. **Authentication Middleware Tests** (Priority: MEDIUM)
TypeScript has detailed auth middleware tests we should match.

**Required Tests:**
- [ ] Bearer token validation
- [ ] Client authentication
- [ ] Allowed methods enforcement
- [ ] Auth error scenarios
- [ ] Token expiration handling
- [ ] Custom auth providers

**Implementation Location:** `server/auth/middleware_test.go`

### 7. **Cross-Spawn Process Management** (Priority: LOW)
TypeScript's `cross-spawn.test.ts` tests process spawning scenarios.

**Required Tests:**
- [ ] Process spawning with arguments
- [ ] Environment variable handling
- [ ] Shell command execution
- [ ] Process cleanup
- [ ] Error propagation

**Implementation Location:** `client/process_test.go`

### 8. **Completable Resources** (Priority: LOW)
TypeScript's `completable.test.ts` tests resource completion features.

**Required Tests:**
- [ ] Basic completion
- [ ] Prefix matching
- [ ] Total limit enforcement
- [ ] Async completion
- [ ] Error handling in completion

**Implementation Location:** `server/completable_test.go`

### 9. **Advanced Client Initialization** (Priority: LOW)
More comprehensive client initialization scenarios from TypeScript.

**Required Tests:**
- [ ] Custom transport configuration
- [ ] Capability negotiation
- [ ] Multiple notification handlers
- [ ] Connection state management
- [ ] Graceful shutdown scenarios

**Implementation Location:** `client/client_init_test.go`

### 10. **Resource Template Support** (Priority: MEDIUM)
TypeScript SDK supports resource templates that we lack.

**Required Implementation:**
- [ ] Resource template registration
- [ ] URI template expansion for resources
- [ ] Template variable validation
- [ ] Dynamic resource generation

**Implementation Location:** `server/resource_template.go` and tests

## Implementation Strategy

### Phase 1: Critical Protocol Features (Week 1-2)
1. URI Template Support
2. Advanced Progress Notifications
3. Protocol Version Negotiation

### Phase 2: Transport Enhancements (Week 3-4)
1. WebSocket Transport
2. Transport Resumability
3. Enhanced connection management

### Phase 3: Advanced Features (Week 5-6)
1. Authentication Middleware
2. Resource Templates
3. Completable Resources

### Phase 4: Polish and Integration (Week 7)
1. Cross-spawn process management
2. Advanced client initialization
3. Integration test suite expansion

## Testing Guidelines

1. **Maintain Race-Free Code**: All new tests must pass with `-race` flag
2. **TypeScript Compatibility**: Ensure behavior matches TypeScript SDK
3. **Benchmark Impact**: Monitor performance impact of new features
4. **Documentation**: Update CLAUDE.md with new patterns

## Success Metrics

- [ ] 100% feature parity with TypeScript SDK tests
- [ ] All tests pass with race detection
- [ ] Maintain 7x+ performance advantage
- [ ] Zero regression in existing functionality
- [ ] Comprehensive documentation updates

## Notes

- The Go SDK already excels in performance and concurrency testing
- Focus should be on protocol compliance and feature completeness
- WebSocket transport is optional but would enhance compatibility
- URI templates are critical for full MCP compliance