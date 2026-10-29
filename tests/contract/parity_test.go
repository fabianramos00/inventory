//go:build contract

package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/bootstrap"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/ports/inbound"
)

const (
	defaultSuperEmail   = "parity-super@example.com"
	defaultSuperPass    = "parity-super-password"
	defaultSuperName    = "Parity Superuser"
	defaultRegularEmail = "parity-regular@example.com"
	defaultRegularPass  = "parity-regular-password"
	defaultRegularName  = "Parity Regular"
)

var moduleOrder = []string{
	"auth/auth.json",
	"users/users.json",
	"inventory/catalog.json",
	"inventory/products.json",
	"clients/clients.json",
	"supply-chain/providers.json",
	"supply-chain/purchase-orders.json",
	"supply-chain/supplier-catalogs.json",
	"sales/sales.json",
	"dashboard/dashboard.json",
}

type fixtureFile struct {
	Module    string    `json:"module"`
	Endpoints int       `json:"endpoints"`
	Fixtures  []fixture `json:"fixtures"`
}

type fixture struct {
	Name          string            `json:"name"`
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	Auth          string            `json:"auth"`
	ContentType   string            `json:"content_type"`
	Body          json.RawMessage   `json:"body"`
	Expect        expectation       `json:"expect"`
	Capture       map[string]string `json:"capture"`
	Compare       string            `json:"compare"`
	StrictCompare string            `json:"strict_compare"`
	OnConflict    *onConflict       `json:"on_conflict"`
}

type expectation struct {
	Status int `json:"status"`
}

type onConflict struct {
	Statuses []int           `json:"statuses"`
	Find     conflictMatcher `json:"find"`
}

type conflictMatcher struct {
	Path    string         `json:"path"`
	ListKey string         `json:"list_key"`
	Match   map[string]any `json:"match"`
}

type seedUser struct {
	Email     string
	Password  string
	FullName  string
	Superuser bool
}

type loginResult struct {
	AccessToken  string
	RefreshToken string
	User         seedUser
}

type side struct {
	name     string
	baseURL  string
	client   *http.Client
	tokens   map[string]string
	vars     map[string]any
	ids      map[string]string
	authUser string
	authPass string
	replay   *replayData
}

type response struct {
	Status int
	Body   []byte
}

type replayData struct {
	Seed      replaySeed                `json:"seed"`
	Responses map[string]replayResponse `json:"responses"`
}

type replaySeed struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Regular  *struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
	} `json:"regular,omitempty"`
}

type replayResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

type skipNote struct {
	Module  string
	Fixture string
	Reason  string
}

type failure struct {
	Module  string
	Fixture string
	Request string
	Details []string
}

func TestContractParity(t *testing.T) {
	files := loadFixtures(t)

	legacyURL := strings.TrimSpace(os.Getenv("LEGACY_API_URL"))
	replayPath := strings.TrimSpace(os.Getenv("CONTRACT_REPLAY_FILE"))
	if replayPath == "" && legacyURL == "" {
		replayPath = filepath.Join(repoRoot(t), "testdata", "contract", "replay", "replay.json")
	}

	var legacy *side
	var super seedUser
	var regular *seedUser
	var recorded map[string]replayResponse

	switch {
	case legacyURL != "":
		if os.Getenv("CONTRACT_ALLOW_WRITES") != "1" {
			t.Skip("contract parity skipped: set CONTRACT_ALLOW_WRITES=1 only for a disposable legacy instance; this harness creates and deletes resources")
		}
		base := normalizeLegacyURL(legacyURL)
		user := strings.TrimSpace(os.Getenv("LEGACY_USER"))
		pass := os.Getenv("LEGACY_PASSWORD")
		if user == "" || pass == "" {
			t.Skip("contract parity skipped: LEGACY_USER and LEGACY_PASSWORD are required")
		}
		superLogin, err := login(base, user, pass)
		if err != nil {
			t.Skipf("contract parity skipped: legacy login failed: %v", err)
		}
		if !superLogin.User.Superuser {
			t.Skip("contract parity skipped: LEGACY_USER must be a superuser")
		}
		super = superLogin.User
		super.Password = pass
		legacy = newLiveSide(base, superLogin.AccessToken, "", user, pass)

		regularUser := strings.TrimSpace(os.Getenv("LEGACY_REGULAR_USER"))
		regularPass := os.Getenv("LEGACY_REGULAR_PASSWORD")
		if regularUser != "" && regularPass != "" && !strings.EqualFold(regularUser, user) {
			regularLogin, err := login(base, regularUser, regularPass)
			if err != nil {
				t.Skipf("contract parity skipped: legacy regular login failed: %v", err)
			}
			regular = &regularLogin.User
			regular.Password = regularPass
			legacy.tokens["user"] = regularLogin.AccessToken
		}
		recorded = map[string]replayResponse{}

	case replayPath != "":
		data, err := os.ReadFile(replayPath)
		if err != nil {
			t.Skipf("contract parity skipped: LEGACY_API_URL is unset and replay file %s is unavailable: %v", replayPath, err)
		}
		var replay replayData
		if err := json.Unmarshal(data, &replay); err != nil {
			t.Fatalf("parse replay file %s: %v", replayPath, err)
		}
		if replay.Seed.Email == "" {
			t.Fatalf("replay file %s has no seed email", replayPath)
		}
		super = seedUser{
			Email:     replay.Seed.Email,
			Password:  envOr("CONTRACT_REPLAY_PASSWORD", defaultSuperPass),
			FullName:  replay.Seed.FullName,
			Superuser: true,
		}
		if replay.Seed.Regular != nil {
			regular = &seedUser{
				Email:    replay.Seed.Regular.Email,
				Password: envOr("CONTRACT_REPLAY_PASSWORD", defaultRegularPass),
				FullName: replay.Seed.Regular.FullName,
			}
		}
		legacy = newReplaySide(&replay)

	default:
		t.Skip("contract parity skipped: set LEGACY_API_URL (disposable instance) or CONTRACT_REPLAY_FILE")
	}

	goSide := startGoServer(t, super, regular)
	goSide.authUser = super.Email
	goSide.authPass = super.Password
	goSide.tokens["superuser"] = loginToken(t, goSide.baseURL, super.Email, super.Password)

	goRegular := regular
	if goRegular == nil {
		goRegular = &seedUser{Email: defaultRegularEmail, Password: defaultRegularPass, FullName: defaultRegularName}
	}
	if !strings.EqualFold(goRegular.Email, super.Email) {
		goSide.tokens["user"] = loginToken(t, goSide.baseURL, goRegular.Email, goRegular.Password)
	}
	if legacy.authUser == "" {
		legacy.authUser = super.Email
		legacy.authPass = super.Password
	}

	var skips []skipNote
	var failures []failure

	for _, file := range files {
		for i := range file.Fixtures {
			f := file.Fixtures[i]
			if f.Auth == "user" && legacy.tokens["user"] == "" {
				skips = append(skips, skipNote{file.Module, f.Name, "legacy regular-user credentials are not configured"})
				continue
			}
			if legacy.replay != nil {
				if _, ok := legacy.replay.Responses[replayKey(file.Module, f.Name)]; !ok {
					skips = append(skips, skipNote{file.Module, f.Name, "no replay recording for this fixture"})
					continue
				}
			}

			goResp, err := execute(goSide, file.Module, f)
			if err != nil {
				failures = append(failures, failure{file.Module, f.Name, requestLine(f), []string{"go request failed: " + err.Error()}})
				continue
			}
			legacyResp, err := execute(legacy, file.Module, f)
			if err != nil {
				failures = append(failures, failure{file.Module, f.Name, requestLine(f), []string{"legacy request failed: " + err.Error()}})
				continue
			}

			if legacy.replay == nil && recorded != nil {
				recorded[replayKey(file.Module, f.Name)] = replayResponse{Status: legacyResp.Status, Body: redactRecorded(legacyResp.Body)}
			}

			if conflict, reason, err := handleConflict(legacy, f, legacyResp); err != nil {
				failures = append(failures, failure{file.Module, f.Name, requestLine(f), []string{err.Error()}})
				continue
			} else if conflict {
				skips = append(skips, skipNote{file.Module, f.Name, reason})
				continue
			}
			if conflict, reason, err := handleConflict(goSide, f, goResp); err != nil {
				failures = append(failures, failure{file.Module, f.Name, requestLine(f), []string{err.Error()}})
				continue
			} else if conflict {
				skips = append(skips, skipNote{file.Module, f.Name, reason})
				continue
			}

			if err := capture(goSide, f, goResp); err != nil {
				failures = append(failures, failure{file.Module, f.Name, requestLine(f), []string{"capture go: " + err.Error()}})
				continue
			}
			if err := capture(legacy, f, legacyResp); err != nil {
				failures = append(failures, failure{file.Module, f.Name, requestLine(f), []string{"capture legacy: " + err.Error()}})
				continue
			}

			mode := f.Compare
			if os.Getenv("CONTRACT_STRICT") == "1" && f.StrictCompare != "" {
				mode = f.StrictCompare
			}
			details := compare(mode, legacySideData(legacy, legacyResp), goSideData(goSide, goResp))
			if len(details) > 0 {
				failures = append(failures, failure{file.Module, f.Name, requestLine(f), details})
			}
		}
	}

	reportPath := filepath.Join(repoRoot(t), "testdata", "contract", "report.md")
	if len(failures) > 0 {
		report := renderReport(legacyLabel(legacyURL, replayPath), failures, skips)
		if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
			t.Fatalf("write parity report: %v", err)
		}
		t.Fatalf("contract parity failed: %d mismatch(es); see %s", len(failures), reportPath)
	}
	_ = os.Remove(reportPath)

	if recordDir := strings.TrimSpace(os.Getenv("CONTRACT_RECORD_DIR")); recordDir != "" && recorded != nil {
		payload := replayData{Seed: replaySeed{Email: super.Email, FullName: super.FullName}, Responses: recorded}
		if regular != nil {
			payload.Seed.Regular = &struct {
				Email    string `json:"email"`
				FullName string `json:"full_name"`
			}{Email: regular.Email, FullName: regular.FullName}
		}
		if err := os.MkdirAll(recordDir, 0o755); err != nil {
			t.Fatalf("create replay directory: %v", err)
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			t.Fatalf("marshal replay recording: %v", err)
		}
		if err := os.WriteFile(filepath.Join(recordDir, "replay.json"), append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write replay recording: %v", err)
		}
	}
}

func loginToken(t *testing.T, base, email, password string) string {
	t.Helper()
	result, err := login(base, email, password)
	if err != nil {
		t.Fatalf("login %s against Go test server: %v", email, err)
	}
	return result.AccessToken
}

func startGoServer(t *testing.T, super seedUser, regular *seedUser) *side {
	t.Helper()
	cfg := &config.Config{
		DBMode:             "sqlite",
		SQLitePath:         filepath.Join(t.TempDir(), "contract.db"),
		SecretKey:          strings.Repeat("p", 32),
		AccessTokenMinutes: 30,
		RefreshTokenDays:   7,
		BcryptRounds:       4,
		AutoMigrate:        true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stores, err := bootstrap.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open Go test stores: %v", err)
	}
	t.Cleanup(func() { _ = stores.Closer() })

	passwords := security.NewPasswordHasher(cfg.BcryptRounds)
	users := application.NewUserService(stores.Users, passwords)
	create := func(seed seedUser) {
		_, err := users.Create(context.Background(), inbound.CreateUserInput{
			Email:       seed.Email,
			Password:    seed.Password,
			FullName:    seed.FullName,
			IsSuperuser: &seed.Superuser,
		})
		if err != nil {
			t.Fatalf("seed Go user %s: %v", seed.Email, err)
		}
	}
	create(super)
	if regular == nil {
		regular = &seedUser{Email: defaultRegularEmail, Password: defaultRegularPass, FullName: defaultRegularName}
	}
	if !strings.EqualFold(regular.Email, super.Email) {
		create(*regular)
	}

	handler := httpapi.NewRouter(httpapi.Deps{
		Auth: application.NewAuthService(
			stores.Users,
			stores.RefreshTokens,
			passwords,
			security.NewAccessTokenIssuer(cfg.SecretKey, cfg.AccessTokenTTL(), "contract"),
			security.NewRefreshTokenService(),
			cfg.RefreshTokenTTL(),
		),
		Users:            users,
		Categories:       application.NewCategoryService(stores.Categories),
		Brands:           application.NewBrandService(stores.Brands),
		Materials:        application.NewMaterialService(stores.Materials),
		MeasurementUnits: application.NewMeasurementUnitService(stores.MeasurementUnits),
		Products:         application.NewProductService(stores.Products),
		Clients:          application.NewClientService(stores.Clients),
		Sales:            application.NewSaleService(stores.Sales, stores.UnitOfWork),
		Providers:        application.NewProviderService(stores.Providers),
		SupplierCatalogs: application.NewSupplierCatalogService(stores.SupplierCatalogs),
		PurchaseOrders:   application.NewPurchaseOrderService(stores.PurchaseOrders, stores.UnitOfWork),
		Dashboard:        application.NewDashboardService(stores.Dashboard),
		Logger:           logger,
		Backend:          stores.Backend,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &side{
		name:     "go",
		baseURL:  server.URL,
		client:   &http.Client{Timeout: 30 * time.Second},
		tokens:   map[string]string{},
		vars:     map[string]any{},
		ids:      map[string]string{},
		authUser: super.Email,
		authPass: super.Password,
	}
}

func newLiveSide(base, superToken, regularToken, user, pass string) *side {
	return &side{
		name:     "legacy",
		baseURL:  base,
		client:   &http.Client{Timeout: 30 * time.Second},
		tokens:   map[string]string{"superuser": superToken, "user": regularToken},
		vars:     map[string]any{},
		ids:      map[string]string{},
		authUser: user,
		authPass: pass,
	}
}

func newReplaySide(replay *replayData) *side {
	return &side{
		name:     "legacy",
		client:   &http.Client{Timeout: 30 * time.Second},
		tokens:   map[string]string{"superuser": "replay-token", "user": "replay-token"},
		vars:     map[string]any{},
		ids:      map[string]string{},
		authUser: replay.Seed.Email,
		authPass: "replay-password",
		replay:   replay,
	}
}

func normalizeLegacyURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	base = strings.TrimSuffix(base, "/api/v1")
	return strings.TrimRight(base, "/")
}

func login(base, email, password string) (*loginResult, error) {
	form := url.Values{"username": {email}, "password": {password}}
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID          int64  `json:"id"`
			FullName    string `json:"full_name"`
			Email       string `json:"email"`
			IsActive    bool   `json:"is_active"`
			IsSuperuser bool   `json:"is_superuser"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode login: %w", err)
	}
	if payload.AccessToken == "" {
		return nil, errors.New("login returned no access token")
	}
	return &loginResult{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		User: seedUser{
			Email:     payload.User.Email,
			FullName:  payload.User.FullName,
			Superuser: payload.User.IsSuperuser,
		},
	}, nil
}

func execute(s *side, module string, f fixture) (response, error) {
	if s.replay != nil {
		recorded, ok := s.replay.Responses[replayKey(module, f.Name)]
		if !ok {
			return response{}, fmt.Errorf("no replay recording for %s", replayKey(module, f.Name))
		}
		return response{Status: recorded.Status, Body: append([]byte(nil), recorded.Body...)}, nil
	}

	path, err := resolveString(f.Path, s)
	if err != nil {
		return response{}, err
	}
	var body io.Reader
	if len(f.Body) > 0 && string(f.Body) != "null" {
		var raw any
		dec := json.NewDecoder(bytes.NewReader(f.Body))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			return response{}, fmt.Errorf("decode fixture body: %w", err)
		}
		resolved, err := resolveValue(raw, s)
		if err != nil {
			return response{}, err
		}
		switch f.ContentType {
		case "form":
			values := url.Values{}
			obj, ok := resolved.(map[string]any)
			if !ok {
				return response{}, errors.New("form body must be an object")
			}
			for key, value := range obj {
				values.Set(key, fmt.Sprint(value))
			}
			body = strings.NewReader(values.Encode())
		default:
			encoded, err := json.Marshal(resolved)
			if err != nil {
				return response{}, err
			}
			body = bytes.NewReader(encoded)
		}
	}

	req, err := http.NewRequest(f.Method, s.baseURL+path, body)
	if err != nil {
		return response{}, err
	}
	if body != nil {
		if f.ContentType == "form" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if f.Auth != "" && f.Auth != "none" {
		token := s.tokens[f.Auth]
		if token == "" {
			return response{}, fmt.Errorf("no %s token", f.Auth)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return response{}, err
	}
	return response{Status: resp.StatusCode, Body: data}, nil
}

func handleConflict(s *side, f fixture, resp response) (bool, string, error) {
	if resp.Status == f.Expect.Status || f.OnConflict == nil {
		return false, "", nil
	}
	allowed := false
	for _, status := range f.OnConflict.Statuses {
		if status == resp.Status {
			allowed = true
			break
		}
	}
	if !allowed {
		return false, "", nil
	}
	if s.replay != nil {
		return false, "", fmt.Errorf("%s returned %d and replay cannot resolve the conflict", replayKey("", f.Name), resp.Status)
	}
	item, err := findConflictItem(s, f.OnConflict.Find)
	if err != nil {
		return false, "", fmt.Errorf("resolve conflict for %s: %w", f.Name, err)
	}
	if err := captureFrom(s, f, item); err != nil {
		return false, "", fmt.Errorf("capture conflict for %s: %w", f.Name, err)
	}
	return true, fmt.Sprintf("fixture already seeded (%d); reused existing resource", resp.Status), nil
}

func findConflictItem(s *side, spec conflictMatcher) (any, error) {
	path, err := resolveString(spec.Path, s)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if token := s.tokens["superuser"]; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lookup status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	list, _ := root.(map[string]any)
	key := spec.ListKey
	if key == "" {
		key = "items"
	}
	items, ok := list[key].([]any)
	if !ok {
		return nil, fmt.Errorf("lookup response has no %q array", key)
	}
	match, err := resolveValue(spec.Match, s)
	if err != nil {
		return nil, err
	}
	wanted, _ := match.(map[string]any)
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		matches := true
		for key, want := range wanted {
			if fmt.Sprint(obj[key]) != fmt.Sprint(want) {
				matches = false
				break
			}
		}
		if matches {
			return obj, nil
		}
	}
	return nil, fmt.Errorf("no item matched %v", wanted)
}

func capture(s *side, f fixture, resp response) error {
	if len(f.Capture) == 0 {
		return nil
	}
	if len(resp.Body) == 0 {
		return errors.New("response body is empty")
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return err
	}
	return captureFrom(s, f, root)
}

func captureFrom(s *side, f fixture, root any) error {
	for name, path := range f.Capture {
		value, ok := lookup(root, path)
		if !ok {
			return fmt.Errorf("capture %s: path %q not found", name, path)
		}
		s.vars[name] = value
		if numeric, ok := numericKey(value); ok {
			s.ids[numeric] = name
		}
	}
	return nil
}

func lookup(root any, path string) (any, bool) {
	current := root
	for _, part := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			value, ok := node[part]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			index := 0
			if _, err := fmt.Sscanf(part, "%d", &index); err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func resolveString(raw string, s *side) (string, error) {
	value, err := resolveValue(raw, s)
	if err != nil {
		return "", err
	}
	return fmt.Sprint(value), nil
}

func resolveValue(value any, s *side) (any, error) {
	switch node := value.(type) {
	case string:
		if strings.HasPrefix(node, "{{") && strings.HasSuffix(node, "}}") {
			key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(node, "{{"), "}}"))
			return resolveVar(key, s)
		}
		resolved := node
		for key, val := range s.vars {
			resolved = strings.ReplaceAll(resolved, "{{"+key+"}}", fmt.Sprint(val))
		}
		return resolved, nil
	case map[string]any:
		out := make(map[string]any, len(node))
		for key, val := range node {
			resolved, err := resolveValue(val, s)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(node))
		for i, val := range node {
			resolved, err := resolveValue(val, s)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return value, nil
	}
}

func resolveVar(key string, s *side) (any, error) {
	switch key {
	case "auth.username":
		return s.authUser, nil
	case "auth.password":
		return s.authPass, nil
	}
	value, ok := s.vars[key]
	if !ok {
		return nil, fmt.Errorf("unresolved placeholder %q", key)
	}
	return value, nil
}

func replayKey(module, name string) string {
	return module + "|" + name
}

func requestLine(f fixture) string {
	return f.Method + " " + f.Path
}

func compare(mode string, legacy, goResp comparisonResponse) []string {
	var details []string
	if legacy.status != goResp.status {
		details = append(details, fmt.Sprintf("status legacy=%d go=%d", legacy.status, goResp.status))
	}
	if mode == "status" {
		return details
	}
	details = append(details, diffValues("$", legacy.body, goResp.body, mode == "shape")...)
	return details
}

type comparisonResponse struct {
	status int
	body   any
}

func legacySideData(s *side, resp response) comparisonResponse {
	return comparisonResponse{status: resp.Status, body: decodeNormalized(resp.Body, s)}
}

func goSideData(s *side, resp response) comparisonResponse {
	return comparisonResponse{status: resp.Status, body: decodeNormalized(resp.Body, s)}
}

func decodeNormalized(body []byte, s *side) any {
	if len(body) == 0 {
		return nil
	}
	var raw any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return string(body)
	}
	return normalize(raw, s, "")
}

func normalize(value any, s *side, key string) any {
	switch node := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, v := range node {
			out[k] = normalize(v, s, k)
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, v := range node {
			out[i] = normalize(v, s, key)
		}
		return out
	case string:
		if key == "full_name" && node == "" {
			return ""
		}
		if strings.Contains(strings.ToLower(key), "token") {
			return "<token>"
		}
		if key == "sku" && strings.HasPrefix(node, "SKU-") {
			return "<generated-sku>"
		}
		if _, err := time.Parse(time.RFC3339, node); err == nil {
			return "<timestamp>"
		}
		return node
	case nil:
		if key == "full_name" {
			return ""
		}
		return nil
	default:
		if key == "id" {
			if _, ok := numericKey(node); ok {
				return "<id>"
			}
		}
		if strings.HasSuffix(key, "_id") {
			if numeric, ok := numericKey(node); ok {
				if name, found := s.ids[numeric]; found {
					return "<" + name + ">"
				}
			}
		}
		return node
	}
}

func diffValues(path string, legacy, goValue any, shape bool) []string {
	if legacy == nil || goValue == nil {
		if legacy == nil && goValue == nil {
			return nil
		}
		return []string{fmt.Sprintf("%s: legacy=%s go=%s", path, render(legacy), render(goValue))}
	}
	switch l := legacy.(type) {
	case map[string]any:
		g, ok := goValue.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: type legacy=object go=%s", path, kind(goValue))}
		}
		var details []string
		for key := range l {
			if _, ok := g[key]; !ok {
				details = append(details, fmt.Sprintf("%s.%s: missing in go", path, key))
			}
		}
		for key := range g {
			if _, ok := l[key]; !ok {
				details = append(details, fmt.Sprintf("%s.%s: extra in go", path, key))
			}
		}
		keys := make([]string, 0, len(l))
		for key := range l {
			if _, ok := g[key]; ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			details = append(details, diffValues(path+"."+key, l[key], g[key], shape)...)
		}
		return details
	case []any:
		g, ok := goValue.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: type legacy=array go=%s", path, kind(goValue))}
		}
		if len(l) != len(g) {
			return []string{fmt.Sprintf("%s: length legacy=%d go=%d", path, len(l), len(g))}
		}
		var details []string
		for i := range l {
			details = append(details, diffValues(fmt.Sprintf("%s[%d]", path, i), l[i], g[i], shape)...)
		}
		return details
	case json.Number:
		g, ok := goValue.(json.Number)
		if !ok {
			return []string{fmt.Sprintf("%s: type legacy=number go=%s", path, kind(goValue))}
		}
		if shape {
			return nil
		}
		ld, lerr := decimal.NewFromString(string(l))
		gd, gerr := decimal.NewFromString(string(g))
		if lerr != nil || gerr != nil {
			if string(l) != string(g) {
				return []string{fmt.Sprintf("%s: legacy=%s go=%s", path, l, g)}
			}
			return nil
		}
		if !ld.Equal(gd) {
			return []string{fmt.Sprintf("%s: legacy=%s go=%s", path, l, g)}
		}
		return nil
	default:
		if shape {
			if kind(legacy) != kind(goValue) {
				return []string{fmt.Sprintf("%s: type legacy=%s go=%s", path, kind(legacy), kind(goValue))}
			}
			return nil
		}
		if fmt.Sprint(legacy) != fmt.Sprint(goValue) {
			return []string{fmt.Sprintf("%s: legacy=%s go=%s", path, render(legacy), render(goValue))}
		}
		return nil
	}
}

func kind(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case json.Number:
		return "number"
	case string:
		return "string"
	case bool:
		return "bool"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func redactRecorded(body []byte) json.RawMessage {
	if len(body) == 0 {
		return nil
	}
	var raw any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return append(json.RawMessage(nil), body...)
	}
	raw = redactTokens(raw, "")
	data, err := json.Marshal(raw)
	if err != nil {
		return append(json.RawMessage(nil), body...)
	}
	return data
}

func redactTokens(value any, key string) any {
	switch node := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, v := range node {
			out[k] = redactTokens(v, k)
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, v := range node {
			out[i] = redactTokens(v, key)
		}
		return out
	case string:
		if strings.Contains(strings.ToLower(key), "token") || strings.HasSuffix(strings.ToLower(key), "_password") {
			return "<redacted>"
		}
		return node
	default:
		return value
	}
}

func render(value any) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func numericKey(value any) (string, bool) {
	switch v := value.(type) {
	case json.Number:
		return v.String(), true
	case float64:
		return fmt.Sprintf("%v", v), true
	case int64:
		return fmt.Sprintf("%d", v), true
	case int:
		return fmt.Sprintf("%d", v), true
	default:
		return "", false
	}
}

func loadFixtures(t *testing.T) []fixtureFile {
	t.Helper()
	root := filepath.Join(repoRoot(t), "testdata", "contract")
	files := make([]fixtureFile, 0, len(moduleOrder))
	for _, rel := range moduleOrder {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixtures %s: %v", path, err)
		}
		var file fixtureFile
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatalf("parse fixtures %s: %v", path, err)
		}
		if file.Module == "" || len(file.Fixtures) == 0 {
			t.Fatalf("fixtures %s: module and fixtures are required", path)
		}
		for i, f := range file.Fixtures {
			if f.Name == "" || f.Method == "" || f.Path == "" || f.Auth == "" || f.Expect.Status == 0 {
				t.Fatalf("fixtures %s[%d]: name, method, path, auth, and expect.status are required", path, i)
			}
		}
		files = append(files, file)
	}
	return files
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate harness source")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("locate repository root")
		}
		dir = parent
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func legacyLabel(liveURL, replayPath string) string {
	if liveURL != "" {
		return normalizeLegacyURL(liveURL)
	}
	return "replay:" + replayPath
}
