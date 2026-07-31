package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRateLimitIdentityUsesAuthenticatedTenantBeforeBFFIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()
	first, _ := gin.CreateTestContext(httptest.NewRecorder())
	first.Request = httptest.NewRequest("GET", "/", nil)
	first.Request.RemoteAddr = "192.0.2.10:4000"
	first.Set(contextUserID, "user-one")
	first.Set(contextAccountID, accountID)

	second, _ := gin.CreateTestContext(httptest.NewRecorder())
	second.Request = httptest.NewRequest("GET", "/", nil)
	second.Request.RemoteAddr = "192.0.2.10:5000"
	second.Set(contextUserID, "user-two")
	second.Set(contextAccountID, uuid.New())

	if got := rateLimitIdentity(first); got != "account:"+accountID.String() {
		t.Fatalf("rateLimitIdentity(first) = %q", got)
	}
	if rateLimitIdentity(first) == rateLimitIdentity(second) {
		t.Fatal("two authenticated tenants sharing one BFF IP received the same rate-limit key")
	}
}

func TestRateLimitIdentityFallsBackFromUserToIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	userContext.Request = httptest.NewRequest("GET", "/", nil)
	userContext.Set(contextUserID, "user-one")
	if got := rateLimitIdentity(userContext); got != "user:user-one" {
		t.Fatalf("rateLimitIdentity(user) = %q", got)
	}

	ipContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ipContext.Request = httptest.NewRequest("GET", "/", nil)
	ipContext.Request.RemoteAddr = "192.0.2.20:4000"
	if got := rateLimitIdentity(ipContext); got != "ip:192.0.2.20" {
		t.Fatalf("rateLimitIdentity(ip) = %q", got)
	}
}
