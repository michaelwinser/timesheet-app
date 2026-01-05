package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/michaelw/timesheet-app/service/internal/store"
)

// MCPOAuthHandler handles OAuth 2.1 endpoints for MCP authentication
type MCPOAuthHandler struct {
	oauthStore *store.MCPOAuthStore
	userStore  *store.UserStore
	jwt        *JWTService
	baseURL    string // e.g., "http://localhost:8080"
}

// NewMCPOAuthHandler creates a new MCP OAuth handler
func NewMCPOAuthHandler(oauthStore *store.MCPOAuthStore, userStore *store.UserStore, jwt *JWTService, baseURL string) *MCPOAuthHandler {
	return &MCPOAuthHandler{
		oauthStore: oauthStore,
		userStore:  userStore,
		jwt:        jwt,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
	}
}

// OAuthMetadata returns OAuth 2.1 Authorization Server Metadata
// GET /.well-known/oauth-authorization-server
func (h *MCPOAuthHandler) OAuthMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]any{
		"issuer":                                h.baseURL,
		"authorization_endpoint":               h.baseURL + "/mcp/authorize",
		"token_endpoint":                       h.baseURL + "/mcp/token",
		"response_types_supported":             []string{"code"},
		"grant_types_supported":                []string{"authorization_code"},
		"code_challenge_methods_supported":     []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		// MCP-specific
		"service_documentation": h.baseURL + "/docs/v2/mcp-usage.md",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// ResourceMetadata returns OAuth 2.0 Protected Resource Metadata
// GET /.well-known/oauth-protected-resource or via WWW-Authenticate header
func (h *MCPOAuthHandler) ResourceMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]any{
		"resource":              h.baseURL + "/mcp",
		"authorization_servers": []string{h.baseURL},
		"bearer_methods_supported": []string{"header"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// Authorize handles the authorization endpoint
// GET /mcp/authorize
func (h *MCPOAuthHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	// Parse OAuth parameters
	responseType := r.URL.Query().Get("response_type")
	redirectURI := r.URL.Query().Get("redirect_uri")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")
	state := r.URL.Query().Get("state") // Client's state parameter

	// Validate required parameters
	if responseType != "code" {
		h.oauthErrorRedirect(w, r, redirectURI, "unsupported_response_type", "Only 'code' response type is supported", state)
		return
	}

	if redirectURI == "" {
		http.Error(w, "redirect_uri is required", http.StatusBadRequest)
		return
	}

	if codeChallenge == "" {
		h.oauthErrorRedirect(w, r, redirectURI, "invalid_request", "code_challenge is required (PKCE)", state)
		return
	}

	if codeChallengeMethod != "S256" {
		h.oauthErrorRedirect(w, r, redirectURI, "invalid_request", "code_challenge_method must be S256", state)
		return
	}

	// Create OAuth session
	session, err := h.oauthStore.CreateSession(r.Context(), codeChallenge, codeChallengeMethod, redirectURI)
	if err != nil {
		h.oauthErrorRedirect(w, r, redirectURI, "server_error", "Failed to create session", state)
		return
	}

	// Store client's state in our session (we'll pass it back in the redirect)
	// For now, we'll include it in the login page as a hidden field

	// Check if user is already authenticated (has valid JWT)
	userID, ok := UserIDFromContext(r.Context())
	if ok {
		// User is already logged in, complete authorization
		authCode, _, err := h.oauthStore.CompleteAuthorization(r.Context(), session.State, userID)
		if err != nil {
			h.oauthErrorRedirect(w, r, redirectURI, "server_error", "Failed to complete authorization", state)
			return
		}

		// Redirect back to MCP client with auth code
		redirectURL, _ := url.Parse(redirectURI)
		q := redirectURL.Query()
		q.Set("code", authCode)
		if state != "" {
			q.Set("state", state)
		}
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	// User is not authenticated, show login page
	h.renderLoginPage(w, session.State, state, redirectURI)
}

// Login handles the login form submission
// POST /mcp/login
func (h *MCPOAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	oauthState := r.FormValue("oauth_state")    // Our session state
	clientState := r.FormValue("client_state")   // Client's state to pass back
	redirectURI := r.FormValue("redirect_uri")

	if email == "" || password == "" {
		h.renderLoginPageWithError(w, oauthState, clientState, redirectURI, "Email and password are required")
		return
	}

	// Validate credentials
	user, err := h.userStore.Authenticate(r.Context(), email, password)
	if err != nil {
		h.renderLoginPageWithError(w, oauthState, clientState, redirectURI, "Invalid email or password")
		return
	}

	// Complete the OAuth authorization
	authCode, _, err := h.oauthStore.CompleteAuthorization(r.Context(), oauthState, user.ID)
	if err != nil {
		if err == store.ErrOAuthSessionNotFound || err == store.ErrOAuthSessionExpired {
			http.Error(w, "Session expired. Please try again.", http.StatusBadRequest)
			return
		}
		http.Error(w, "Authorization failed", http.StatusInternalServerError)
		return
	}

	// Redirect back to MCP client with auth code
	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "Invalid redirect URI", http.StatusBadRequest)
		return
	}

	q := redirectURL.Query()
	q.Set("code", authCode)
	if clientState != "" {
		q.Set("state", clientState)
	}
	redirectURL.RawQuery = q.Encode()
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// AuthorizeWithToken handles authorization when user provides their JWT
// This is an alternative flow where the user pastes their JWT
// POST /mcp/authorize
func (h *MCPOAuthHandler) AuthorizeWithToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token               string `json:"token"`
		CodeChallenge       string `json:"code_challenge"`
		CodeChallengeMethod string `json:"code_challenge_method"`
		RedirectURI         string `json:"redirect_uri"`
		State               string `json:"state"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate JWT
	userID, err := h.jwt.ValidateToken(req.Token)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "access_denied",
			"error_description": "Invalid token",
		})
		return
	}

	// Create session and complete authorization
	session, err := h.oauthStore.CreateSession(r.Context(), req.CodeChallenge, req.CodeChallengeMethod, req.RedirectURI)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	authCode, _, err := h.oauthStore.CompleteAuthorization(r.Context(), session.State, userID)
	if err != nil {
		http.Error(w, "Failed to complete authorization", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"code":         authCode,
		"redirect_uri": req.RedirectURI,
		"state":        req.State,
	})
}

// Token handles the token exchange endpoint
// POST /mcp/token
func (h *MCPOAuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.tokenError(w, "invalid_request", "Failed to parse form")
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")

	if grantType != "authorization_code" {
		h.tokenError(w, "unsupported_grant_type", "Only authorization_code grant is supported")
		return
	}

	if code == "" {
		h.tokenError(w, "invalid_request", "code is required")
		return
	}

	if codeVerifier == "" {
		h.tokenError(w, "invalid_request", "code_verifier is required (PKCE)")
		return
	}

	// Exchange auth code for access token
	token, err := h.oauthStore.ExchangeAuthCode(r.Context(), code, codeVerifier)
	if err != nil {
		switch err {
		case store.ErrInvalidAuthCode:
			h.tokenError(w, "invalid_grant", "Invalid authorization code")
		case store.ErrAuthCodeExpired:
			h.tokenError(w, "invalid_grant", "Authorization code expired")
		case store.ErrCodeChallengeInvalid:
			h.tokenError(w, "invalid_grant", "Code verifier does not match challenge")
		default:
			h.tokenError(w, "server_error", "Failed to exchange code")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": token.Token,
		"token_type":   "Bearer",
		"expires_in":   86400, // 24 hours in seconds
	})
}

func (h *MCPOAuthHandler) oauthErrorRedirect(w http.ResponseWriter, r *http.Request, redirectURI, errorCode, description, state string) {
	if redirectURI == "" {
		http.Error(w, fmt.Sprintf("%s: %s", errorCode, description), http.StatusBadRequest)
		return
	}

	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}

	q := redirectURL.Query()
	q.Set("error", errorCode)
	q.Set("error_description", description)
	if state != "" {
		q.Set("state", state)
	}
	redirectURL.RawQuery = q.Encode()
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

func (h *MCPOAuthHandler) tokenError(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

func (h *MCPOAuthHandler) renderLoginPage(w http.ResponseWriter, oauthState, clientState, redirectURI string) {
	h.renderLoginPageWithError(w, oauthState, clientState, redirectURI, "")
}

func (h *MCPOAuthHandler) renderLoginPageWithError(w http.ResponseWriter, oauthState, clientState, redirectURI, errorMsg string) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Authorize MCP Access - Timesheet</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0f172a;
            color: #e2e8f0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 1rem;
        }
        .container {
            background: #1e293b;
            border-radius: 12px;
            padding: 2rem;
            width: 100%;
            max-width: 400px;
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
        }
        h1 {
            font-size: 1.5rem;
            margin-bottom: 0.5rem;
        }
        .subtitle {
            color: #94a3b8;
            margin-bottom: 1.5rem;
        }
        .info-box {
            background: #334155;
            border-radius: 8px;
            padding: 1rem;
            margin-bottom: 1.5rem;
            font-size: 0.875rem;
        }
        .info-box strong {
            color: #60a5fa;
        }
        .error {
            background: #7f1d1d;
            border: 1px solid #dc2626;
            border-radius: 6px;
            padding: 0.75rem;
            margin-bottom: 1rem;
            font-size: 0.875rem;
        }
        form { display: flex; flex-direction: column; gap: 1rem; }
        label {
            font-size: 0.875rem;
            color: #94a3b8;
            margin-bottom: 0.25rem;
            display: block;
        }
        input[type="email"], input[type="password"] {
            width: 100%;
            padding: 0.75rem;
            border: 1px solid #475569;
            border-radius: 6px;
            background: #0f172a;
            color: #e2e8f0;
            font-size: 1rem;
        }
        input:focus {
            outline: none;
            border-color: #60a5fa;
        }
        button {
            padding: 0.75rem 1.5rem;
            background: #3b82f6;
            color: white;
            border: none;
            border-radius: 6px;
            font-size: 1rem;
            cursor: pointer;
            margin-top: 0.5rem;
        }
        button:hover { background: #2563eb; }
        .cancel {
            text-align: center;
            margin-top: 1rem;
        }
        .cancel a {
            color: #94a3b8;
            text-decoration: none;
        }
        .cancel a:hover { color: #e2e8f0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Authorize MCP Access</h1>
        <p class="subtitle">An AI assistant is requesting access to your timesheet.</p>

        <div class="info-box">
            <strong>Claude Code</strong> wants to:
            <ul style="margin-top: 0.5rem; margin-left: 1rem;">
                <li>View your projects</li>
                <li>View and create time entries</li>
                <li>Classify calendar events</li>
            </ul>
        </div>

        {{if .Error}}<div class="error">{{.Error}}</div>{{end}}

        <form method="POST" action="/mcp/login">
            <input type="hidden" name="oauth_state" value="{{.OAuthState}}">
            <input type="hidden" name="client_state" value="{{.ClientState}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">

            <div>
                <label for="email">Email</label>
                <input type="email" id="email" name="email" required autofocus>
            </div>
            <div>
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            <button type="submit">Authorize</button>
        </form>

        <div class="cancel">
            <a href="{{.RedirectURI}}?error=access_denied&error_description=User+denied+access{{if .ClientState}}&state={{.ClientState}}{{end}}">Cancel</a>
        </div>
    </div>
</body>
</html>`

	t, err := template.New("login").Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := map[string]string{
		"OAuthState":  oauthState,
		"ClientState": clientState,
		"RedirectURI": redirectURI,
		"Error":       errorMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, data)
}
