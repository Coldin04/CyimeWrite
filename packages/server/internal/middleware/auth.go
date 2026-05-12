package middleware

import (
	"strings"

	"g.co1d.in/Coldin04/Cyime/server/internal/auth"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func jwtKeyFunc(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fiber.NewError(
			fiber.StatusUnauthorized,
			"unexpected signing method: "+token.Header["alg"].(string),
		)
	}
	// Single source of truth — no inline fallback. If JWT_SECRET_KEY is missing
	// or weak, every request fails fast with the same error the token issuer
	// would have raised at startup.
	return auth.LoadJWTSecret()
}

func parseJWT(tokenString string) (*auth.JWTClaims, error) {
	claims := &auth.JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, jwtKeyFunc)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid JWT")
	}
	return claims, nil
}

func parseBearerToken(c *fiber.Ctx) (string, error) {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return "", fiber.NewError(fiber.StatusUnauthorized, "Missing or malformed JWT")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", fiber.NewError(
			fiber.StatusUnauthorized,
			"Malformed Authorization header, expected 'Bearer {token}'",
		)
	}
	return parts[1], nil
}

func parseJWTFromRequest(c *fiber.Ctx) (*auth.JWTClaims, error) {
	tokenString, err := parseBearerToken(c)
	if err != nil {
		return nil, err
	}
	return parseJWT(tokenString)
}

func isMediaContentCookieRequest(c *fiber.Ctx) bool {
	if c.Method() != fiber.MethodGet {
		return false
	}
	path := strings.TrimRight(c.Path(), "/")
	return strings.HasSuffix(path, "/content") || strings.HasSuffix(path, "/thumbnail")
}

func parseJWTFromMediaContentRequest(c *fiber.Ctx) (*auth.JWTClaims, error) {
	if strings.TrimSpace(c.Get("Authorization")) != "" {
		tokenString, err := parseBearerToken(c)
		if err != nil {
			return nil, err
		}
		return parseJWT(tokenString)
	}

	// Media content and thumbnail URLs are loaded directly by browsers (for
	// example via <img>), so these read-only endpoints may use the media cookie.
	// Keep this fallback out of the generic Protected middleware so metadata and
	// state-changing media APIs remain Authorization-header only.
	if !isMediaContentCookieRequest(c) {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Missing or malformed JWT")
	}
	tokenString := strings.TrimSpace(c.Cookies("cyime_media_access_token"))
	if tokenString == "" {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Missing or malformed JWT")
	}
	return parseJWT(tokenString)
}

// Protected is a middleware that protects routes requiring a valid JWT.
// It verifies the token and passes the userId to the next handler via c.Locals().
func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := parseJWTFromRequest(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Invalid or expired JWT",
				"details": err.Error(),
			})
		}
		c.Locals("userId", claims.UserID.String())
		return c.Next()
	}
}

// ProtectedMediaContent protects read-only media content routes. It accepts the
// normal Authorization bearer token and, only for GET content/thumbnail routes,
// the HttpOnly media cookie used by browser media elements.
func ProtectedMediaContent() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := parseJWTFromMediaContentRequest(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Invalid or expired JWT",
				"details": err.Error(),
			})
		}
		c.Locals("userId", claims.UserID.String())
		return c.Next()
	}
}

// OptionalProtected parses JWT when provided, but does not block anonymous requests.
func OptionalProtected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if strings.TrimSpace(authHeader) == "" {
			return c.Next()
		}

		claims, err := parseJWTFromRequest(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Invalid or expired JWT",
				"details": err.Error(),
			})
		}
		c.Locals("userId", claims.UserID.String())
		return c.Next()
	}
}
