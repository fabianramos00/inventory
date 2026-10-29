package httpapi_test

import (
	"net/http"
	"strconv"
	"testing"
)

func TestDashboardTopProductsLimitBoundsAccepted(t *testing.T) {
	service := &stubDashboardService{}
	server := newDashboardTestServer(t, service)

	for _, limit := range []string{"1", "100"} {
		t.Run("limit "+limit, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/dashboard/top-products?limit="+limit, "user", nil)
			if status != http.StatusOK {
				t.Fatalf("limit %s status = %d body = %s, want 200", limit, status, raw)
			}
			want, err := strconv.Atoi(limit)
			if err != nil {
				t.Fatal(err)
			}
			if service.topLimit != want {
				t.Fatalf("limit passed to service = %d, want %d", service.topLimit, want)
			}
		})
	}
}
