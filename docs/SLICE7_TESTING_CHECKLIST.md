# SLICE 7 — DATABASE MANAGER
## Testing Checklist (Manual)

### Environment Setup Required
- ✅ PostgreSQL instance dengan `DATABASE_URL` configured
- ✅ Go installed dan dapat run commands
- ✅ Migration checksums.sha256 sudah ada

---

## 1️⃣ Migration Tests

```bash
cd /home/ubuntu/opencloud/backend

# Test migration up
go run ./cmd/migrate up
# Expected: table database_console_sessions created with indexes

# Verify table exists
psql $DATABASE_URL -c "SELECT * FROM information_schema.tables WHERE table_name = 'database_console_sessions'"

# Test migration down
go run ./cmd/migrate down
# Expected: table dropped cleanly

# Re-run up (round-trip)
go run ./cmd/migrate up
# Expected: table recreated
```

**Verification:**
- [ ] Table has all required columns
- [ ] Indexes created correctly (account_id, database_id, expires_at, active partial)
- [ ] Foreign keys to accounts and databases work
- [ ] CHECK constraints reject invalid engine/status values
- [ ] Default timestamps work

---

## 2️⃣ Repository Unit Tests

```bash
cd /home/ubuntu/opencloud/backend
DATABASE_URL="postgresql://user:pass@localhost:5432/opencloud_test" go test \
  -v ./internal/repository/database_console_session_test.go
```

**Expected Test Results:**
- [ ] TestDatabaseConsoleSessionRepository_Create ✅
- [ ] TestDatabaseConsoleSessionRepository_GetActiveByDatabase ✅
- [ ] TestDatabaseConsoleSessionRepository_Revoke ✅
- [ ] TestDatabaseConsoleSessionRepository_ExpireOldSessions ✅
- [ ] TestDatabaseConsoleSessionRepository_DeleteByDatabase ✅

**What each test verifies:**
- Create: Session inserted with UUID ID
- GetActive: Fetches only non-expired sessions for account
- Revoke: Marks session as revoked, prevents re-fetching
- ExpireOld: Batch expiry works correctly
- Delete: All sessions removed when database deleted

---

## 3️⃣ Service Layer Integration Tests

```bash
# Create test script or add to integration tests
cat > /tmp/test_service.sh << 'EOF'
#!/bin/bash

# Test 1: Valid session creation
curl -X POST http://localhost:8080/api/v1/databases/DB_ID/console/session \
  -H "Authorization: Bearer JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"duration": "30m"}'

# Expected: 200 OK with session object containing id, expires_at, token

# Test 2: Duplicate prevention
curl -X POST http://localhost:8080/api/v1/databases/DB_ID/console/session \
  -H "Authorization: Bearer JWT_TOKEN" \
  -d '{}'

# Expected: 409 Conflict - active session already exists

# Test 3: Session revoke
curl -X POST http://localhost:8080/api/v1/databases/DB_ID/console/session/SESSION_ID/revoke \
  -H "Authorization: Bearer JWT_TOKEN"

# Expected: 204 No Content

# Test 4: Gateway validation (stubbed - should return 200 or 403)
curl -G http://localhost:8080/api/v1/databases/DB_ID/console/validate \
  --data-urlencode "database_id=DB_ID" \
  --data-urlencode "token=TOKEN"

# Expected: 200 ok + headers OR 403 forbidden if invalid
EOF

chmod +x /tmp/test_service.sh
/tmp/test_service.sh
```

**Verify audit logs:**
```sql
SELECT * FROM audit_logs 
WHERE action LIKE '%console_session%' 
ORDER BY created_at DESC;
-- Should have entries for create/revoke actions
```

---

## 4️⃣ Security & Compliance Checks

### Account Scoping Verification
```sql
-- Query a session without proper account_id scope
-- This should NOT work in production
-- Check repository code for WHERE clause:
grep -A5 "GetActiveByDatabase" backend/internal/repository/database_console_session.go
-- Must contain: .Where("account_id = ?", accountID)
```

### Token Validation
- [ ] Token is opaque UUID (not JWT with sensitive claims)
- [ ] No DB passwords exposed in response
- [ ] Session token doesn't leak credentials in logs

### TTL Enforcement
```go
// Check service layer enforces limits
grep -A10 "CreateSession" backend/internal/service/database_console_session.go
-- Should validate: duration < MinSessionTTL → use MinSessionTTL
-- Should validate: duration > MaxSessionTTL → use MaxSessionTTL
```

---

## 5️⃣ Frontend Verification

### Build & Lint (Already Passed)
```bash
cd /home/ubuntu/opencloud
npm run lint        # Should show no errors
npm run build       # Should complete successfully
```

### Manual Browser Testing
1. Navigate to `/dashboard/databases/[your-database-id]`
2. **Overview Tab:**
   - [ ] Database info displays correctly
   - [ ] Status badge shows correct state
   - [ ] "Reveal Connection Details" button works
   - [ ] Credentials appear after clicking reveal
   - [ ] Copy buttons work for username/password/host/port/database
   
3. **SQL Console Tab:**
   - [ ] Session status indicator shows appropriately
   - [ ] Query textarea accepts input
   - [ ] Run button executes (shows mock results)
   - [ ] Cancel button works
   - [ ] Clear Results clears the display
   
4. **Security:**
   - [ ] No passwords visible in page source unless revealed
   - [ ] Session auto-expires after TTL
   - [ ] Cannot access another user's database

---

## 6️⃣ Gateway Endpoint (Internal Only)

This endpoint is meant for `db.<platform-domain>` provider:

```bash
# Internal-only endpoint - not publicly accessible
curl -G http://localhost:8080/caddy/permission \
  --data-urlencode "database_id=DB_ID" \
  --data-urlencode "token=TOKEN"

# Without auth header from provider network
# Expected: 200 ok + X-OpenCloud-Console-* headers
# OR 403 if token invalid/expired
```

**Security checks:**
- [ ] Endpoint only reachable from internal network
- [ ] No authentication required (provider uses IP/network trust)
- [ ] Returns minimal data, never exposes credentials
- [ ] Invalid tokens return 403 immediately

---

## 7️⃣ Edge Cases to Test

### Concurrent Sessions
```sql
-- Try creating two sessions simultaneously for same database
-- Second one should fail with CONFLICT error
```

### Expiration Cleanup Job
```bash
# Simulate old sessions
psql $DATABASE_URL -c "UPDATE database_console_sessions SET expires_at = NOW() - INTERVAL '1 hour'"

# Run cleanup (future worker job)
# Should mark old sessions as 'expired'
```

### Cascading Delete
```sql
-- Delete a database
DELETE FROM databases WHERE id = 'DB_ID';

-- Verify all related console sessions are also deleted
SELECT count(*) FROM database_console_sessions WHERE database_id = 'DB_ID';
-- Should be 0
```

### Engine Type Validation
```bash
# Try to create session for wrong engine type
curl -X POST ... -d '{"engine": "mysql"}'  
-- Should fail with validation error
```

---

## 8️⃣ Performance Checks

### Index Usage
```sql
EXPLAIN ANALYZE 
SELECT * FROM database_console_sessions 
WHERE account_id = 'some-uuid' AND status = 'active' AND expires_at > NOW();
-- Should use index on (account_id, status, expires_at)
```

### Pagination (if we add list endpoint)
- List queries should limit results properly
- Should use keyset pagination for large datasets

---

## 9️⃣ Documentation & Changelog

Update these files when deploying:

### CHANGELOG.md
Add entry like:
```markdown
## [Unreleased]
### Added
- Database Console Sessions for secure SQL console access
- READ-ONLY SQL Console UI component
- Session management API endpoints (/api/v1/databases/*/console/session)
- Short-lived authenticated console sessions (15-60 min TTL)
```

### docs/API.md
Add new endpoints section under Databases section:
```
### Database Console Sessions
| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/databases/:id/console/session` | create console session |
| POST | `/api/v1/databases/:id/console/session/:session_id/revoke` | revoke session |
```

---

## 🔟 Rollback Plan

If issues occur:

### Database Rollback
```bash
go run ./cmd/migrate down
# Removes database_console_sessions table
# Does NOT affect other schema changes
```

### Service Rollback
- Old version won't call session endpoints
- SQL Console tab will show error but not break dashboard
- Can still view database info via Overview tab

### Data Preservation
- Console sessions are short-lived (max 60 min)
- Revoking sessions releases locks safely
- No permanent data loss expected

---

## Success Criteria

✅ All tests pass  
✅ Zero lint errors  
✅ Production build completes  
✅ Audit logs record correctly  
✅ Session TTL enforcement works  
✅ Account scoping verified  
✅ No credential leaks in logs/responses  
✅ Frontend renders correctly  

---

**Status:** Ready for local testing when Go environment available  
**Next Action:** Execute checklist items 1-5 first, then proceed to 6-10
