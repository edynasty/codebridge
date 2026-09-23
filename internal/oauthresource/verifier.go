package oauthresource

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
)

type Config struct {
	Issuer   string
	Audience string
	JWKSURL  string
}

type Verifier struct {
	issuer   string
	audience string
	jwksURL  string
	client   *http.Client

	mu        sync.RWMutex
	keys      map[string]signingKey
	fetchedAt time.Time
	cacheTTL  time.Duration
}

type signingKey struct {
	Key any
	Alg string
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg,omitempty"`
	Use string `json:"use,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
}

func New(cfg Config) (*Verifier, error) {
	cfg.Issuer = strings.TrimSpace(cfg.Issuer)
	cfg.Audience = strings.TrimSpace(cfg.Audience)
	cfg.JWKSURL = strings.TrimSpace(cfg.JWKSURL)
	if cfg.Issuer == "" || cfg.Audience == "" || cfg.JWKSURL == "" {
		return nil, errors.New("issuer, audience and jwks URL are required")
	}
	if err := requireHTTPSOrLoopback(cfg.Issuer); err != nil {
		return nil, fmt.Errorf("issuer: %w", err)
	}
	if err := requireHTTPSOrLoopback(cfg.JWKSURL); err != nil {
		return nil, fmt.Errorf("jwks URL: %w", err)
	}
	return &Verifier{
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
		jwksURL:  cfg.JWKSURL,
		client:   &http.Client{Timeout: 10 * time.Second},
		keys:     map[string]signingKey{},
		cacheTTL: 15 * time.Minute,
	}, nil
}

func (v *Verifier) Verify(ctx context.Context, tokenString string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token header is missing kid")
		}
		key, err := v.key(ctx, kid)
		if err != nil {
			return nil, err
		}
		if key.Alg != "" && key.Alg != token.Method.Alg() {
			return nil, fmt.Errorf("token algorithm %q does not match jwk algorithm %q", token.Method.Alg(), key.Alg)
		}
		return key.Key, nil
	})
	if err != nil || token == nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
	}

	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return nil, fmt.Errorf("%w: missing expiration", mcpauth.ErrInvalidToken)
	}
	sub, _ := claims.GetSubject()
	scopes := extractScopes(claims)
	return &mcpauth.TokenInfo{
		Scopes:     scopes,
		Expiration: exp.Time,
		UserID:     sub,
		Extra: map[string]any{
			"issuer":   v.issuer,
			"audience": v.audience,
		},
	}, nil
}

func (v *Verifier) key(ctx context.Context, kid string) (signingKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	age := time.Since(v.fetchedAt)
	v.mu.RUnlock()

	if ok && age <= v.cacheTTL {
		return key, nil
	}
	// Unknown kids are untrusted input. Once we have a fresh JWKS, do not let
	// random kid values trigger an upstream fetch on every request.
	if !ok && age <= 30*time.Second {
		return signingKey{}, fmt.Errorf("no signing key for kid %q", kid)
	}
	// Fail closed when the cache is stale: if the authorization server cannot
	// refresh its keys, do not continue trusting an indefinitely old key set.
	if err := v.refresh(ctx); err != nil {
		return signingKey{}, err
	}
	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return signingKey{}, fmt.Errorf("no signing key for kid %q", kid)
	}
	return key, nil
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: unexpected HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var doc jwksDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}
	keys := make(map[string]signingKey, len(doc.Keys))
	for _, item := range doc.Keys {
		if item.Kid == "" || (item.Use != "" && item.Use != "sig") {
			continue
		}
		key, err := parseJWK(item)
		if err != nil {
			continue
		}
		keys[item.Kid] = signingKey{Key: key, Alg: item.Alg}
	}
	if len(keys) == 0 {
		return errors.New("jwks contained no usable signing keys")
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func parseJWK(j jwk) (any, error) {
	switch j.Kty {
	case "RSA":
		nBytes, err := base64.RawURLEncoding.DecodeString(j.N)
		if err != nil || len(nBytes) == 0 {
			return nil, errors.New("invalid RSA modulus")
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(j.E)
		if err != nil || len(eBytes) == 0 || len(eBytes) > 4 {
			return nil, errors.New("invalid RSA exponent")
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		if e < 3 {
			return nil, errors.New("invalid RSA exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
	case "EC":
		curve := curveForName(j.Crv)
		if curve == nil {
			return nil, fmt.Errorf("unsupported EC curve %q", j.Crv)
		}
		xBytes, err := base64.RawURLEncoding.DecodeString(j.X)
		if err != nil {
			return nil, errors.New("invalid EC x coordinate")
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(j.Y)
		if err != nil {
			return nil, errors.New("invalid EC y coordinate")
		}
		x, y := new(big.Int).SetBytes(xBytes), new(big.Int).SetBytes(yBytes)
		if !curve.IsOnCurve(x, y) {
			return nil, errors.New("EC key is not on the declared curve")
		}
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
	default:
		return nil, fmt.Errorf("unsupported jwk key type %q", j.Kty)
	}
}

func curveForName(name string) elliptic.Curve {
	switch name {
	case "P-256":
		return elliptic.P256()
	case "P-384":
		return elliptic.P384()
	case "P-521":
		return elliptic.P521()
	default:
		return nil
	}
}

func extractScopes(claims jwt.MapClaims) []string {
	var raw []string
	appendScope := func(v any) {
		switch x := v.(type) {
		case string:
			raw = append(raw, strings.Fields(x)...)
		case []string:
			raw = append(raw, x...)
		case []any:
			for _, item := range x {
				if s, ok := item.(string); ok {
					raw = append(raw, strings.Fields(s)...)
				}
			}
		}
	}
	appendScope(claims["scope"])
	appendScope(claims["scp"])
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func requireHTTPSOrLoopback(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return errors.New("invalid URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return errors.New("must use https (http is allowed only for loopback development)")
}
