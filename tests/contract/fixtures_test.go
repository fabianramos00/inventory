//go:build contract

package contract

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var expectedEndpoints = map[string][]string{
	"auth": {
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
	},
	"users": {
		"GET /api/v1/users",
		"POST /api/v1/users",
		"GET /api/v1/users/{id}",
		"PUT /api/v1/users/{id}",
		"DELETE /api/v1/users/{id}",
	},
	"inventory/catalog": {
		"GET /api/v1/inventory/categories",
		"POST /api/v1/inventory/categories",
		"GET /api/v1/inventory/categories/{id}",
		"PUT /api/v1/inventory/categories/{id}",
		"DELETE /api/v1/inventory/categories/{id}",
		"GET /api/v1/inventory/materials",
		"POST /api/v1/inventory/materials",
		"PUT /api/v1/inventory/materials/{id}",
		"DELETE /api/v1/inventory/materials/{id}",
		"GET /api/v1/inventory/brands",
		"POST /api/v1/inventory/brands",
		"GET /api/v1/inventory/brands/{id}",
		"PUT /api/v1/inventory/brands/{id}",
		"DELETE /api/v1/inventory/brands/{id}",
		"GET /api/v1/inventory/measurement-units",
		"POST /api/v1/inventory/measurement-units",
		"PUT /api/v1/inventory/measurement-units/{id}",
		"DELETE /api/v1/inventory/measurement-units/{id}",
	},
	"inventory/products": {
		"GET /api/v1/inventory/products",
		"POST /api/v1/inventory/products",
		"GET /api/v1/inventory/products/{id}",
		"PUT /api/v1/inventory/products/{id}",
		"DELETE /api/v1/inventory/products/{id}",
		"GET /api/v1/inventory/products/stats/overview",
	},
	"clients": {
		"GET /api/v1/sales/clients",
		"POST /api/v1/sales/clients",
		"GET /api/v1/sales/clients/{id}",
		"PUT /api/v1/sales/clients/{id}",
		"DELETE /api/v1/sales/clients/{id}",
	},
	"supply-chain/providers": {
		"GET /api/v1/supply-chain/providers",
		"POST /api/v1/supply-chain/providers",
		"GET /api/v1/supply-chain/providers/{id}",
		"PUT /api/v1/supply-chain/providers/{id}",
		"DELETE /api/v1/supply-chain/providers/{id}",
	},
	"supply-chain/supplier-catalogs": {
		"GET /api/v1/supply-chain/supplier-catalogs",
		"GET /api/v1/supply-chain/supplier-catalogs/{id}",
	},
	"supply-chain/purchase-orders": {
		"GET /api/v1/supply-chain/purchase-orders",
		"POST /api/v1/supply-chain/purchase-orders",
		"GET /api/v1/supply-chain/purchase-orders/{id}",
		"PUT /api/v1/supply-chain/purchase-orders/{id}",
		"DELETE /api/v1/supply-chain/purchase-orders/{id}",
		"GET /api/v1/supply-chain/purchase-orders/{id}/items",
		"POST /api/v1/supply-chain/purchase-orders/{id}/items",
		"PUT /api/v1/supply-chain/purchase-orders/{id}/items/{id}",
		"DELETE /api/v1/supply-chain/purchase-orders/{id}/items/{id}",
	},
	"sales": {
		"GET /api/v1/sales",
		"POST /api/v1/sales",
		"GET /api/v1/sales/stats",
		"GET /api/v1/sales/{id}",
		"PUT /api/v1/sales/{id}",
		"DELETE /api/v1/sales/{id}",
		"GET /api/v1/sales/{id}/items",
		"POST /api/v1/sales/{id}/items",
		"PUT /api/v1/sales/{id}/items/{id}",
		"DELETE /api/v1/sales/{id}/items/{id}",
	},
	"dashboard": {
		"GET /api/v1/dashboard/summary",
		"GET /api/v1/dashboard/sales-trend",
		"GET /api/v1/dashboard/top-products",
		"GET /api/v1/dashboard/order-status-distribution",
	},
}

var placeholderPattern = regexp.MustCompile(`\{\{[^}]+\}\}`)

func TestFixtureCoverage(t *testing.T) {
	files := loadFixtures(t)
	covered := map[string]map[string]bool{}
	total := 0
	for _, file := range files {
		expected, ok := expectedEndpoints[file.Module]
		if !ok {
			t.Fatalf("unexpected fixture module %q", file.Module)
		}
		if file.Endpoints != len(expected) {
			t.Errorf("module %s declares %d endpoints, expected %d", file.Module, file.Endpoints, len(expected))
		}
		total += file.Endpoints
		covered[file.Module] = map[string]bool{}
		for _, f := range file.Fixtures {
			covered[file.Module][canonicalEndpoint(f.Method, f.Path)] = true
		}
	}
	if total != 67 {
		t.Errorf("fixtures declare %d endpoints, expected 67", total)
	}

	for module, expected := range expectedEndpoints {
		got := covered[module]
		for _, endpoint := range expected {
			if !got[endpoint] {
				t.Errorf("module %s missing endpoint %s", module, endpoint)
			}
		}
		extra := make([]string, 0)
		expectedSet := map[string]bool{}
		for _, endpoint := range expected {
			expectedSet[endpoint] = true
		}
		for endpoint := range got {
			if !expectedSet[endpoint] {
				extra = append(extra, endpoint)
			}
		}
		sort.Strings(extra)
		for _, endpoint := range extra {
			t.Errorf("module %s has unlisted endpoint %s", module, endpoint)
		}
	}
}

func canonicalEndpoint(method, path string) string {
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	path = placeholderPattern.ReplaceAllString(path, "{id}")
	return strings.ToUpper(method) + " " + path
}
